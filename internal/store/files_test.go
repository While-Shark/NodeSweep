package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatePermissionsAndLinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		p := path + suffix
		if err := os.WriteFile(p, nil, 0644); err != nil {
			t.Fatal(err)
		}
		if err := secureStateFile(p, false); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(p)
		if info.Mode().Perm() != 0600 {
			t.Fatal("exposed state", suffix)
		}
	}
	target := filepath.Join(dir, "secret")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.db")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link); err == nil {
		t.Fatal("opened database symlink")
	}
	hard := filepath.Join(dir, "hard.db")
	if err := os.Link(target, hard); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(hard); err == nil {
		t.Fatal("opened hardlinked database")
	}
}
func TestDatabaseRejectsWritableDirectoryAndEscapesURI(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(dir, "unsafe.db")); err == nil {
		t.Fatal("opened database in shared writable directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state?mode=ro.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatal("URI metacharacters changed actual database path", err)
	}
}
