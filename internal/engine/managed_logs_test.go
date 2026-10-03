package engine

import (
	"context"
	"path/filepath"
	"testing"
)

func TestManagedLogsAlwaysExcludedAndExecutionRechecks(t *testing.T) {
	root := t.TempDir()
	e := testEngine(root)
	managed := filepath.Join(root, "managed")
	e.managedRoots = func() []string { return []string{managed} }
	oldFile(t, root, "managed/unsafe.log.1")
	oldFile(t, root, "safe.log.1")
	oldFile(t, root, "container-json.log.1.gz")
	oldFile(t, root, "system.journal.gz")
	rule := rule(root)
	rule.Excludes = nil
	plan, err := e.preview(context.Background(), rule)
	if err != nil || len(plan.Files) != 1 || plan.Files[0].Path != "safe.log.1" {
		t.Fatal(plan, err)
	}
	nested := rule
	nested.Root = managed
	if _, err = e.preview(context.Background(), nested); err == nil {
		t.Fatal("explicit managed root allowed")
	}
	e.managedRoots = func() []string { return []string{root} }
	result, err := e.execute(context.Background(), plan.ID)
	if err == nil || result.Deleted != 0 {
		t.Fatal("changed managed root not checked", result, err)
	}
	if _, err = e.execute(context.Background(), plan.ID); err == nil {
		t.Fatal("plan replay")
	}
	for _, path := range []string{"/var/log/journal", "/var/log/journal/a", "/var/lib/docker/containers/x"} {
		if !managedLogPath(path, []string{"/var/log/journal", "/var/lib/docker/containers"}) {
			t.Fatal(path)
		}
	}
	if managedLogPath("/var/log/journals", []string{"/var/log/journal"}) {
		t.Fatal("prefix boundary")
	}
}
