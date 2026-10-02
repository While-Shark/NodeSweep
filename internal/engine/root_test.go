package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedRootRejectsSymlinkAfterEarlierCheck(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "logs")
	outside := t.TempDir()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := noSymlinks(path); err != nil {
		t.Fatal(err)
	}
	// Deterministically replace the root after the old validation step.
	if err := os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if root, err := openDirectory(path); err == nil {
		root.Close()
		t.Fatal("followed replaced allowlist root")
	}
}
func TestPinnedRootSurvivesReplacementAndRejectsEscapes(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "logs")
	outside := t.TempDir()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	oldFile(t, path, "old.log.1")
	oldFile(t, outside, "secret.log.1")
	root, err := openDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat("old.log.1"); err != nil {
		t.Fatal("lost pinned directory", err)
	}
	if _, err := root.Stat("secret.log.1"); err == nil {
		t.Fatal("redirected pinned root")
	}
	if _, err := root.Stat("../secret.log.1"); err == nil {
		t.Fatal("allowed parent traversal")
	}
}
