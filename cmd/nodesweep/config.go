package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
)

// Open without following a secret-bearing config symlink or blocking on a FIFO.
func readConfig(path string) (Config, error) {
	var config Config
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return config, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return config, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || st.Nlink != 1 {
		return config, errors.New("configuration must be a regular file without links")
	}
	if info.Mode().Perm()&0077 != 0 {
		return config, errors.New("configuration contains credentials: run chmod 600 on the configuration file")
	}
	if info.Size() > 256<<10 {
		return config, errors.New("configuration exceeds size limit")
	}
	decoder := json.NewDecoder(io.LimitReader(file, (256<<10)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, errors.New("invalid configuration JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return config, errors.New("configuration must contain one JSON object")
	}
	return config, nil
}
func validCredential(value string) bool {
	return len(value) >= 32 && len(value) <= 256 && !strings.ContainsAny(value, " \t\r\n")
}
