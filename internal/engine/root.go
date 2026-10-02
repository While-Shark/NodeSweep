package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// Pin each path component with O_NOFOLLOW. A check followed by OpenRoot(path)
// would allow a replaced component to redirect the operation outside its allowlist.
// /proc/self/fd refers to our still-owned descriptor, never an agent-supplied path.
func openDirectory(path string) (*pinnedRoot, error) {
	if !filepath.IsAbs(path) || len(path) > 4096 || strings.ContainsRune(path, 0) {
		return nil, errors.New("absolute path required")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	retained := false
	defer func() {
		if !retained {
			_ = unix.Close(fd)
		}
	}()
	for _, component := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if component == "" {
			continue
		}
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, fmt.Errorf("cannot open directory without symlinks: %w", err)
		}
		_ = unix.Close(fd)
		fd = next
	}
	root, err := os.OpenRoot(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return nil, err
	}
	retained = true
	return &pinnedRoot{Root: root, fd: fd}, nil
}

type pinnedRoot struct {
	*os.Root
	fd       int
	once     sync.Once
	closeErr error
}

func (r *pinnedRoot) Close() error {
	r.once.Do(func() { r.closeErr = r.Root.Close(); _ = unix.Close(r.fd) })
	return r.closeErr
}
