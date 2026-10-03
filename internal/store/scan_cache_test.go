package store

import (
	"github.com/While-Shark/NodeSweep/internal/engine"
	"testing"
	"time"
)

func TestRetainedScanIsNodePathScopedAndNeverCreatesTasks(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	for i, v := range []struct{ node, path, status string }{{"a", "/logs", "failed"}, {"b", "/logs", "succeeded"}, {"a", "/else", "succeeded"}, {"a", "/logs", "running"}} {
		task := engine.Task{ID: engine.ID(), Node: v.node, Created: now.Add(time.Duration(i) * time.Second), Status: v.status, Request: engine.Request{Kind: "scan", Path: v.path}, Result: engine.Scan{Tree: &engine.Entry{Path: v.path}, Truncated: v.status == "failed", At: now}}
		if err := s.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	found, err := s.LatestScan("a", "/logs")
	if err != nil || found == nil || found.Status != "failed" || found.Node != "a" || found.Request.Path != "/logs" {
		t.Fatal(found, err)
	}
	if found, err = s.LatestScan("missing", "/logs"); err != nil || found != nil {
		t.Fatal(found, err)
	}
	tasks, err := s.Tasks()
	if err != nil || len(tasks) != 4 {
		t.Fatal("cache created work", len(tasks), err)
	}
	if _, err = s.DB.Exec(`UPDATE tasks SET body=json_remove(body,'$.result') WHERE node='a' AND json_extract(body,'$.request.path')='/logs'`); err != nil {
		t.Fatal(err)
	}
	if found, err = s.LatestScan("a", "/logs"); err != nil || found != nil {
		t.Fatal("pruned scan reused", found, err)
	}
}
