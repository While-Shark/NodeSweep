package store

import (
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"testing"
	"time"
)

func TestScanCancellationAndProgressOwnership(t *testing.T) {
	s := testStore(t)
	makeTask := func(id, node, kind string) {
		if err := s.CreateTask(engine.Task{ID: id, Node: node, Status: "pending", Created: time.Now(), Request: engine.Request{Kind: kind}}); err != nil {
			t.Fatal(err)
		}
	}
	makeTask("scan", "a", "scan")
	makeTask("delete", "b", "execute")
	if _, err := s.CancelScan("delete"); err == nil {
		t.Fatal("destructive cancellation accepted")
	}
	if _, err := s.Claim("scan", "a"); err != nil {
		t.Fatal(err)
	}
	p := engine.ScanProgress{Visited: 100, Files: 80, Limit: 1000}
	if err := s.ScanProgress("scan", "b", p); err == nil {
		t.Fatal("cross-node progress accepted")
	}
	if err := s.ScanProgress("scan", "a", p); err != nil {
		t.Fatal(err)
	}
	p.Visited = 1
	p.Files = 1
	s.ScanProgress("scan", "a", p)
	task, _ := s.Task("scan")
	if task.Progress == nil || task.Progress.Visited != 100 {
		t.Fatal("stale progress overwritten")
	}
	task, err := s.CancelScan("scan")
	if err != nil || !task.CancelRequested || task.Status != "running" || s.ScanCancellation("b") != "" || s.ScanCancellation("a") != "scan" {
		t.Fatal(task, err)
	}
	if err := s.Complete(engine.Task{ID: "scan", Node: "a", Status: "failed", Error: "context canceled"}); err != nil {
		t.Fatal(err)
	}
	makeTask("queued", "c", "scan")
	task, err = s.CancelScan("queued")
	if err != nil || task.Status != "failed" {
		t.Fatal(task, err)
	}
	next, err := s.Next("c")
	if err != nil || next != nil {
		t.Fatal("cancelled queue replayed")
	}
}

func TestCompactCancelledScanPayloads(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("scan-%02d", i)
		if err := s.CreateTask(engine.Task{ID: id, Node: id, Status: "pending", Created: time.Now().Add(time.Duration(i) * time.Second), Request: engine.Request{Kind: "scan"}}); err != nil {
			t.Fatal(err)
		}
		s.Claim(id, id)
		if err := s.Complete(engine.Task{ID: id, Node: id, Status: "failed", Result: map[string]any{"partial": "tree"}, Error: "context canceled"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CompactScans(); err != nil {
		t.Fatal(err)
	}
	old, _ := s.Task("scan-00")
	latest, _ := s.Task("scan-11")
	if old.Result != nil || latest.Result == nil || old.Error != "context canceled" {
		t.Fatal("failed scan payload retention is unbounded")
	}
}
