package store

import (
	"github.com/While-Shark/NodeSweep/internal/engine"
	"path/filepath"
	"testing"
	"time"
)

func TestRestartDoesNotReplay(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db")
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	task := engine.Task{ID: "pending", Node: "remote", Status: "running", Created: time.Now(), Request: engine.Request{Kind: "execute"}}
	if e = s.SaveTask(task); e != nil {
		t.Fatal(e)
	}
	s.DB.Close()
	s, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	got, e := s.Task(task.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.Status != "interrupted" {
		t.Fatal(got.Status)
	}
	next, e := s.Next("remote")
	if e != nil || next != nil {
		t.Fatal("replayed interrupted task", e)
	}
}
