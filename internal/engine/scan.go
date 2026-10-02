package engine

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

func (e *Engine) scan(ctx context.Context, path string) (Scan, error) {
	out := Scan{At: time.Now()}
	p, err := allowed(path, e.ScanRoots)
	if err != nil {
		return out, err
	}
	if err = noSymlinks(p); err != nil {
		return out, err
	}
	root, err := os.OpenRoot(p)
	if err != nil {
		return out, err
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil {
		return out, err
	}
	device := info.Sys().(*syscall.Stat_t).Dev
	out.Tree = &Entry{Name: filepath.Base(p), Path: p, Directory: true}
	dirs := map[string]*Entry{".": out.Tree}
	seen := map[[2]uint64]bool{}
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			out.Skipped++
			return nil
		}
		if rel == "." {
			return nil
		}
		if out.Files >= 100000 {
			out.Truncated = true
			return fs.SkipAll
		}
		if d.Type()&os.ModeSymlink != 0 {
			out.Skipped++
			return nil
		}
		i, er := d.Info()
		if er != nil {
			out.Skipped++
			return nil
		}
		st := i.Sys().(*syscall.Stat_t)
		if st.Dev != device {
			out.Skipped++
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Virtual/system directories are not meaningful disk trees.
		abs := filepath.Join(p, rel)
		if d.IsDir() && (abs == "/proc" || abs == "/sys" || abs == "/dev" || abs == "/run") {
			out.Skipped++
			return fs.SkipDir
		}
		if !d.IsDir() && !i.Mode().IsRegular() {
			return nil
		}
		out.Files++
		n := &Entry{Name: d.Name(), Path: abs, Directory: d.IsDir()}
		if d.IsDir() {
			dirs[rel] = n
		} else {
			key := [2]uint64{st.Dev, st.Ino}
			if !seen[key] {
				n.Bytes = st.Blocks * 512
				seen[key] = true
			}
		}
		parent := dirs[filepath.Dir(rel)]
		if parent != nil {
			parent.Children = append(parent.Children, n)
		}
		return nil
	})
	var total func(*Entry) int64
	total = func(n *Entry) int64 {
		for _, c := range n.Children {
			n.Bytes += total(c)
		}
		sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Bytes > n.Children[j].Bytes })
		return n.Bytes
	}
	total(out.Tree)
	return out, err
}
