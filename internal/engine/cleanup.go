package engine

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Open descriptors are checked before preview and execution. Failure to inspect
// a process fails closed: incomplete /proc visibility cannot establish safety.
func openFiles() (map[[2]uint64]bool, error) {
	out := map[[2]uint64]bool{}
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	for _, p := range procs {
		if _, err := strconv.Atoi(p.Name()); err != nil {
			continue
		}
		fds, err := os.ReadDir("/proc/" + p.Name() + "/fd")
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot inspect process %s: %w", p.Name(), err)
		}
		for _, fd := range fds {
			i, err := os.Stat("/proc/" + p.Name() + "/fd/" + fd.Name())
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if st, ok := i.Sys().(*syscall.Stat_t); ok {
				out[[2]uint64{st.Dev, st.Ino}] = true
			}
		}
	}
	return out, nil
}
func (e *Engine) preview(ctx context.Context, r Rule) (Plan, error) {
	plan := Plan{ID: ID(), Rule: r, Created: time.Now(), Files: []Candidate{}}
	if err := ValidateRule(r); err != nil {
		return plan, err
	}
	p, err := allowed(r.Root, e.Roots)
	if err != nil {
		return plan, err
	}
	if p == "/" {
		return plan, errors.New("filesystem root cannot be a cleanup root")
	}
	if err = noSymlinks(p); err != nil {
		return plan, err
	}
	root, err := os.OpenRoot(p)
	if err != nil {
		return plan, err
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil {
		return plan, err
	}
	dev := info.Sys().(*syscall.Stat_t).Dev
	busy, err := e.inspectOpen()
	if err != nil {
		return plan, err
	}
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 100000 {
			return errors.New("preview exceeds 100000 entries; choose a narrower directory")
		}
		if rel == "." {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".nodesweep-") || match(d.Name(), r.Excludes) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		i, err := d.Info()
		if err != nil {
			return err
		}
		st := i.Sys().(*syscall.Stat_t)
		if st.Dev != dev {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !i.Mode().IsRegular() || st.Nlink != 1 {
			return nil
		}
		// v0.1 only deletes archived logs; live .log files never qualify.
		if !archived(d.Name()) || !match(d.Name(), r.Patterns) || !i.ModTime().Before(plan.Created.Add(-time.Duration(r.KeepDays)*24*time.Hour)) || busy[[2]uint64{st.Dev, st.Ino}] {
			return nil
		}
		c := Candidate{Path: rel, Size: i.Size(), Modified: i.ModTime().UnixNano(), Inode: st.Ino, Device: st.Dev}
		if len(plan.Files) >= 5000 {
			return errors.New("more than 5000 candidates; narrow the rule before cleanup")
		}
		plan.Files = append(plan.Files, c)
		plan.Bytes += st.Blocks * 512
		return nil
	})
	if err != nil {
		return plan, err
	}
	for id, p := range e.plans {
		if time.Since(p.Created) > 10*time.Minute {
			delete(e.plans, id)
		}
	}
	if len(e.plans) >= 20 {
		return plan, errors.New("too many previews; wait for expiry")
	}
	e.plans[plan.ID] = plan
	return plan, nil
}
func archived(name string) bool {
	if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".xz") || strings.HasSuffix(name, ".bz2") {
		return true
	}
	idx := strings.LastIndex(name, ".log.")
	if idx < 0 {
		return false
	}
	suffix := name[idx+5:]
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
func (e *Engine) execute(ctx context.Context, id string) (CleanupResult, error) {
	result := CleanupResult{Skipped: []string{}}
	p, ok := e.plans[id]
	if !ok {
		return result, errors.New("preview missing, expired or already consumed")
	}
	delete(e.plans, id) // At-most-once: retries require a fresh preview.
	if time.Since(p.Created) > 10*time.Minute {
		return result, errors.New("preview expired")
	}
	path, err := allowed(p.Rule.Root, e.Roots)
	if err != nil {
		return result, err
	}
	if err = noSymlinks(path); err != nil {
		return result, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return result, err
	}
	defer root.Close()
	busy, err := e.inspectOpen()
	if err != nil {
		return result, err
	}
	for _, c := range p.Files {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err := noSymlinks(filepath.Join(path, c.Path)); err != nil {
			result.Skipped = append(result.Skipped, c.Path+": path changed")
			continue
		}
		i, err := root.Lstat(c.Path)
		if err != nil {
			result.Skipped = append(result.Skipped, c.Path+": unavailable")
			continue
		}
		st := i.Sys().(*syscall.Stat_t)
		if !i.Mode().IsRegular() || st.Nlink != 1 || st.Ino != c.Inode || st.Dev != c.Device || i.Size() != c.Size || i.ModTime().UnixNano() != c.Modified || busy[[2]uint64{st.Dev, st.Ino}] {
			result.Skipped = append(result.Skipped, c.Path+": changed or open")
			continue
		}
		if err = removeVerified(root, c, e.inspectOpen); err != nil {
			result.Skipped = append(result.Skipped, c.Path+": "+err.Error())
			continue
		}
		result.Deleted++
		result.Bytes += st.Blocks * 512
	}
	return result, nil
}

// Atomically move the directory entry into a private directory before verifying
// its identity. A replaced leaf must never be unlinked after a stale Lstat.
func removeVerified(root *os.Root, c Candidate, inspect func() (map[[2]uint64]bool, error)) error {
	parent, err := root.OpenRoot(filepath.Dir(c.Path))
	if err != nil {
		return err
	}
	defer parent.Close()
	quarantine := ".nodesweep-" + ID()
	if err = parent.Mkdir(quarantine, 0700); err != nil {
		return err
	}
	defer parent.Remove(quarantine)
	q, err := parent.OpenRoot(quarantine)
	if err != nil {
		return err
	}
	defer q.Close()
	if err = parent.Rename(filepath.Base(c.Path), quarantine+"/item"); err != nil {
		return err
	}
	restore := func(reason error) error {
		// Do not overwrite a replacement created by the application in the meantime.
		from, e := q.Open(".")
		if e != nil {
			return fmt.Errorf("%v; retained in %s/item", reason, quarantine)
		}
		defer from.Close()
		to, e := parent.Open(".")
		if e != nil {
			return fmt.Errorf("%v; retained in %s/item", reason, quarantine)
		}
		defer to.Close()
		e = unix.Renameat2(int(from.Fd()), "item", int(to.Fd()), filepath.Base(c.Path), unix.RENAME_NOREPLACE)
		if e != nil {
			return fmt.Errorf("%v; retained in %s/item (restore: %v)", reason, quarantine, e)
		}
		return reason
	}
	i, err := q.Lstat("item")
	if err != nil {
		return restore(err)
	}
	st := i.Sys().(*syscall.Stat_t)
	if !i.Mode().IsRegular() || st.Nlink != 1 || st.Ino != c.Inode || st.Dev != c.Device || i.Size() != c.Size || i.ModTime().UnixNano() != c.Modified {
		return restore(errors.New("file identity changed"))
	}
	busy, err := inspect()
	if err != nil {
		return restore(err)
	}
	if busy[[2]uint64{st.Dev, st.Ino}] {
		return restore(errors.New("file is open"))
	}
	if err = q.Remove("item"); err != nil {
		return restore(err)
	}
	return nil
}
