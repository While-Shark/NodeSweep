package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNodeLocalPlansCannotAuthorizeAnotherEngine(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	oldFile(t, a, "archive.log.1")
	oldFile(t, b, "archive.log.1")
	first, second := testEngine(a), testEngine(b)
	one, err := first.preview(context.Background(), rule(a))
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.preview(context.Background(), rule(b))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = second.execute(context.Background(), one.ID); err == nil {
		t.Fatal("cross-node plan accepted")
	}
	if _, err = os.Stat(filepath.Join(b, "archive.log.1")); err != nil {
		t.Fatal("wrong node changed", err)
	}
	for _, job := range []struct {
		engine *Engine
		id     string
	}{{first, one.ID}, {second, two.ID}} {
		result, err := job.engine.execute(context.Background(), job.id)
		if err != nil || result.Deleted != 1 {
			t.Fatal(result, err)
		}
		if _, err = job.engine.execute(context.Background(), job.id); err == nil {
			t.Fatal("plan replay accepted")
		}
	}
}
