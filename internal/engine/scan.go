package engine

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func (e *Engine) scan(ctx context.Context, path string) (Scan, error) {
	out := Scan{At: time.Now()}
	p, err := allowed(path, e.ScanRoots)
	if err != nil {
		return out, err
	}
	root, err := openDirectory(p)
	if err != nil {
		return out, err
	}
	defer root.Close()
	fd, err := unix.Openat(root.fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return out, err
	}
	file := os.NewFile(uintptr(fd), p)
	defer file.Close()
	var base unix.Stat_t
	if err = unix.Fstat(fd, &base); err != nil {
		return out, err
	}
	budget := e.ScanBudget.defaults()
	work, cancel := context.WithTimeout(ctx, time.Duration(budget.Seconds)*time.Second)
	defer cancel()
	out.Tree = &Entry{Name: filepath.Base(p), Path: p, Directory: true}
	seen := map[[2]uint64]bool{}
	retained := 128 + 6*(len(p)+len(out.Tree.Name))
	if retained > budget.TreeBytes {
		out.Tree = nil
		return out, errors.New("scan root exceeds result size budget")
	}
	visited := 0
	var allocated int64
	lastProgress := time.Time{}
	observer, _ := ctx.Value(scanObserver{}).(func(ScanProgress))
	publish := func(force bool) {
		if observer != nil && (force || time.Since(lastProgress) >= 250*time.Millisecond) {
			lastProgress = time.Now()
			observer(ScanProgress{Visited: visited, Files: out.Files, Bytes: allocated, Limit: budget.Entries, ElapsedMillis: time.Since(out.At).Milliseconds()})
		}
	}
	publish(true)
	defer func() { publish(true) }()
	limited := errors.New("scan budget reached")
	hit := func(reason string) error {
		out.Truncated = true
		if out.Reason == "" {
			out.Reason = reason
		}
		return limited
	}
	var walk func(*os.File, *Entry, int) error
	walk = func(dir *os.File, parent *Entry, depth int) error {
		for {
			if work.Err() != nil {
				return work.Err()
			}
			entries, readErr := dir.ReadDir(256) // Never allocate an entire wide directory listing.
			for _, entry := range entries {
				if work.Err() != nil {
					return work.Err()
				}
				if visited >= budget.Entries {
					return hit("entries")
				}
				visited++
				if visited%128 == 0 {
					publish(false)
					timer := time.NewTimer(time.Duration(budget.PauseMillis) * time.Millisecond)
					select {
					case <-work.Done():
						timer.Stop()
						return work.Err()
					case <-timer.C:
					}
				}
				var st unix.Stat_t
				if err := unix.Fstatat(int(dir.Fd()), entry.Name(), &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
					out.Skipped++
					continue
				}
				kind := st.Mode & unix.S_IFMT
				if st.Dev != base.Dev || (kind != unix.S_IFDIR && kind != unix.S_IFREG) {
					out.Skipped++
					continue
				}
				abs := filepath.Join(parent.Path, entry.Name())
				if kind == unix.S_IFDIR && (abs == "/proc" || abs == "/sys" || abs == "/dev" || abs == "/run") {
					out.Skipped++
					continue
				}
				cost := 128 + 6*(len(abs)+len(entry.Name())) // Conservative worst-case JSON escaping bound.
				if retained+cost > budget.TreeBytes {
					return hit("tree_bytes")
				}
				retained += cost
				node := &Entry{Name: entry.Name(), Path: abs, Directory: kind == unix.S_IFDIR}
				parent.Children = append(parent.Children, node)
				out.Files++
				if kind == unix.S_IFREG {
					key := [2]uint64{uint64(st.Dev), st.Ino}
					if !seen[key] {
						node.Bytes = st.Blocks * 512
						allocated += node.Bytes
						seen[key] = true
					}
					continue
				}
				if depth >= budget.Depth {
					out.Skipped++
					out.Truncated = true
					if out.Reason == "" {
						out.Reason = "depth"
					}
					continue
				}
				childFD, err := unix.Openat(int(dir.Fd()), entry.Name(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				if err != nil {
					out.Skipped++
					continue
				}
				child := os.NewFile(uintptr(childFD), abs)
				var actual unix.Stat_t
				if err := unix.Fstat(childFD, &actual); err != nil || actual.Dev != st.Dev || actual.Ino != st.Ino {
					child.Close()
					out.Skipped++
					continue
				}
				err = walk(child, node, depth+1)
				child.Close()
				if err != nil {
					return err
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					out.Skipped++
				}
				return nil
			}
		}
	}
	err = walk(file, out.Tree, 1)
	if errors.Is(err, limited) {
		err = nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		out.Truncated = true
		out.Reason = "time"
		if ctx.Err() == nil {
			err = nil
		}
	}
	if errors.Is(err, context.Canceled) {
		out.Truncated = true
		out.Reason = "cancelled"
	}
	var total func(*Entry) int64
	total = func(node *Entry) int64 {
		for _, child := range node.Children {
			node.Bytes += total(child)
		}
		sort.Slice(node.Children, func(i, j int) bool { return node.Children[i].Bytes > node.Children[j].Bytes })
		return node.Bytes
	}
	total(out.Tree)
	return out, err
}
