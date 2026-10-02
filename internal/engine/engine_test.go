package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func oldFile(t *testing.T, root, name string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("archived log\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
}
func rule(root string) Rule {
	return Rule{Name: "test", Root: root, Patterns: []string{"*"}, Excludes: []string{"excluded"}, KeepDays: 14}
}
func TestPreviewExecuteSafety(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	e := testEngine(root)
	oldFile(t, root, "app.log.1")
	oldFile(t, root, "live.log")
	oldFile(t, root, "excluded/old.log.1")
	oldFile(t, outside, "external.log.1")
	oldFile(t, root, "held.log.1")
	oldFile(t, root, "linked.log.1")
	if err := os.Link(filepath.Join(root, "linked.log.1"), filepath.Join(root, "linked.log.2")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(filepath.Join(root, "held.log.1"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	p, err := e.preview(context.Background(), rule(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || p.Files[0].Path != "app.log.1" {
		t.Fatalf("unexpected candidates: %+v", p.Files)
	}
	result, err := e.execute(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("%+v", result)
	}
	if _, err = e.execute(context.Background(), p.ID); err == nil {
		t.Fatal("replayed plan")
	}
	for _, p := range []string{filepath.Join(root, "live.log"), filepath.Join(root, "held.log.1"), filepath.Join(outside, "external.log.1")} {
		if _, err = os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
}
func TestChangedFileAndExpiry(t *testing.T) {
	root := t.TempDir()
	e := testEngine(root)
	oldFile(t, root, "a.log.1")
	p, err := e.preview(context.Background(), rule(root))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "a.log.1"), []byte("new application content"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := e.execute(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Deleted != 0 || len(r.Skipped) != 1 {
		t.Fatalf("%+v", r)
	}
	p.Created = time.Now().Add(-11 * time.Minute)
	e.plans[p.ID] = p
	if _, err = e.execute(context.Background(), p.ID); err == nil {
		t.Fatal("expired preview accepted")
	}
}
func TestPathBoundaries(t *testing.T) {
	root := t.TempDir()
	e := testEngine(root)
	for _, p := range []string{root + "-other", "/", filepath.Join(root, ".."), "relative"} {
		if _, err := e.preview(context.Background(), rule(p)); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
	if err := os.Symlink(root, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.preview(context.Background(), rule(filepath.Join(root, "alias"))); err == nil {
		t.Fatal("accepted symlink root")
	}
}
func TestScanAllocatedAndSymlinks(t *testing.T) {
	root := t.TempDir()
	oldFile(t, root, "nested/a.log.1")
	if err := os.Symlink("/", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	e := New(nil, []string{root})
	s, err := e.scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if s.Tree.Bytes <= 0 || s.Files != 2 || s.Skipped != 1 {
		t.Fatalf("%+v", s)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = e.scan(ctx, root); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestRenameVerificationRestoresReplacement(t *testing.T) {
	root := t.TempDir()
	oldFile(t, root, "a.log.1")
	e := testEngine(root)
	p, err := e.preview(context.Background(), rule(root))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, "a.log.1")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "a.log.1"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err = removeVerified(r, p.Files[0], e.inspectOpen); err == nil {
		t.Fatal("removed replacement")
	}
	b, err := os.ReadFile(filepath.Join(root, "a.log.1"))
	if err != nil || string(b) != "replacement" {
		t.Fatal("replacement not restored", err)
	}
}

// CI sandboxes often prohibit reading unrelated processes. Exercise the actual
// descriptor/inode logic against this test process; production still fails closed
// when any process descriptor cannot be inspected.
func testEngine(root string) *Engine {
	e := New([]string{root}, []string{root})
	e.inspectOpen = func() (map[[2]uint64]bool, error) {
		out := map[[2]uint64]bool{}
		fds, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			return nil, err
		}
		for _, fd := range fds {
			info, err := os.Stat("/proc/self/fd/" + fd.Name())
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			st := info.Sys().(*syscall.Stat_t)
			out[[2]uint64{st.Dev, st.Ino}] = true
		}
		return out, nil
	}
	return e
}

func TestUnreadableProcessesFailClosed(t *testing.T) {
	root := t.TempDir()
	oldFile(t, root, "a.log.1")
	e := testEngine(root)
	e.inspectOpen = func() (map[[2]uint64]bool, error) { return nil, errors.New("permission denied") }
	if _, err := e.preview(context.Background(), rule(root)); err == nil {
		t.Fatal("ignored unavailable process visibility")
	}
	if _, err := os.Stat(filepath.Join(root, "a.log.1")); err != nil {
		t.Fatal("deleted without visibility")
	}
}
