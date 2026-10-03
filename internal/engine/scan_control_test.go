package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func scanFixture(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(root, strconv.Itoa(i)+".log"), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestScanBudgetsAndCancellation(t *testing.T) {
	root := scanFixture(t, 1500)
	e := New(nil, []string{root})
	e.ScanBudget = ScanBudget{Entries: 37}
	out, err := e.scan(context.Background(), root)
	if err != nil || !out.Truncated || out.Reason != "entries" || out.Files != 37 {
		t.Fatalf("limit lost %+v %v", out, err)
	}
	e.ScanBudget = ScanBudget{TreeBytes: 4096}
	out, err = e.scan(context.Background(), root)
	raw, _ := json.Marshal(out)
	if err != nil || !out.Truncated || out.Reason != "tree_bytes" || len(raw) > 4096 {
		t.Fatal("payload budget", len(raw), out.Reason, err)
	}
	e.ScanBudget = ScanBudget{PauseMillis: 100, Seconds: 1}
	out, err = e.scan(context.Background(), root)
	if err != nil || !out.Truncated || out.Reason != "time" {
		t.Fatal("time budget", out.Reason, err)
	}
	e.ScanBudget = ScanBudget{PauseMillis: 100}
	ctx, cancel := context.WithCancel(context.Background())
	last := ScanProgress{}
	ctx = WithScanProgress(ctx, func(p ScanProgress) {
		last = p
		if p.Visited >= 128 {
			cancel()
		}
	})
	start := time.Now()
	out, err = e.scan(ctx, root)
	if !errors.Is(err, context.Canceled) || out.Reason != "cancelled" || out.Files < 1 || last.Visited < 128 || time.Since(start) > time.Second {
		t.Fatalf("cancel lost: %+v %+v %v", out, last, err)
	}
	if ValidateScanBudget(ScanBudget{Entries: 100001}) == nil || ValidateScanBudget(ScanBudget{Depth: 65}) == nil {
		t.Fatal("oversized local budget accepted")
	}
}
func TestScanDepthAndSymlinkBounds(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	os.MkdirAll(deep, 0700)
	os.WriteFile(filepath.Join(deep, "deep.log"), []byte("x"), 0600)
	outside := scanFixture(t, 3)
	os.Symlink(outside, filepath.Join(root, "escape"))
	e := New(nil, []string{root})
	e.ScanBudget = ScanBudget{Depth: 2}
	out, err := e.scan(context.Background(), root)
	if err != nil || !out.Truncated || out.Reason != "depth" || out.Skipped < 2 {
		t.Fatal(out, err)
	}
	raw, _ := json.Marshal(out)
	if string(raw) == "" || out.Tree.Bytes != 0 {
		t.Fatal("symlink/deep files included")
	}
}
