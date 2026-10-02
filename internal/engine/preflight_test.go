package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightRejectsSymlinksAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	e := New([]string{root, link}, []string{filepath.Join(root, "missing")})
	e.inspectOpen = func() (map[[2]uint64]bool, error) { return nil, errors.New("blocked") }
	checks := e.CheckEnvironment()
	if len(checks) != 4 || !checks[0].OK || checks[1].OK || checks[2].OK || checks[3].OK {
		t.Fatalf("wrong checks: %+v", checks)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatal("preflight changed directory", err)
	}
}
