package store

import (
	"errors"
	"os"
	"syscall"
)

func secureStateFile(path string, create bool) error {
	flags := os.O_RDWR | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		flags |= os.O_CREATE
	}
	file, err := os.OpenFile(path, flags, 0600)
	if !create && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || st.Nlink != 1 || int(st.Uid) != os.Geteuid() {
		return errors.New("database state must be an owned regular file without links")
	}
	return file.Chmod(0600)
}
