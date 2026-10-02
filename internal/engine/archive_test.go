package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveNaming(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"app.log", false}, {"app.log.", false}, {"app.log.--", false},
		{"app.log.1", true}, {"app.log.2026-09-01", true}, {"app.log.backup", false},
		{"app.log.1.gz", true}, {"app.log.1.xz", true}, {"app.log.1.bz2", true},
		{"app-2024-02-29T23-59-59.123.log", true},
		{"app-core-2026-09-01T01-02-03.000.log", true},
		{"app-2025-02-29T23-59-59.123.log", false},
		{"app-2026-09-01T25-02-03.000.log", false},
		{"app-2026-09-01T01-02-03.log", false},
		{"app-2026-09-01T01-02-03.0000.log", false},
		{"-2026-09-01T01-02-03.000.log", false},
		{"app-2026-09-01.log", false}, {"app-2026-09-01T01-02-03.000.log.tmp", false},
	} {
		if got := archived(tc.name); got != tc.want {
			t.Errorf("archived(%q)=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTimestampArchivesPreserveCurrentInvalidAndOpenedLogs(t *testing.T) {
	root := t.TempDir()
	names := []string{"panel-2026-01-01T00-00-00.000.log", "panel-2026-01-02T00-00-00.000.log", "panel-2026-02-30T00-00-00.000.log", "panel.log", "panel-2026-01-03T00-00-00.000.log"}
	for _, name := range names {
		oldFile(t, root, name)
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Join(root, names[4]), now, now); err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(filepath.Join(root, names[1]))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	e := testEngine(root)
	r := rule(root)
	r.Patterns = []string{timestampLogPattern}
	plan, err := e.preview(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != names[0] {
		t.Fatalf("unexpected candidates: %+v", plan.Files)
	}
	result, err := e.execute(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, name := range names[1:] {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("preserved %s: %v", name, err)
		}
	}
}
