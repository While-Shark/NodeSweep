package alerts

import (
	"context"
	"encoding/json"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupFailuresOptInDeduplicateAndRedact(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	s, err := New(db.DB, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	db.AddNode(store.Node{ID: "one", Name: "VPS"}, "")
	for _, job := range []engine.Task{
		{ID: "failed", Node: "one", Status: "failed", Created: now, Request: engine.Request{Kind: "execute"}, Error: "secret raw error and /private/path"},
		{ID: "scan", Node: "one", Status: "failed", Created: now, Request: engine.Request{Kind: "scan"}},
		{ID: "success", Node: "one", Status: "succeeded", Created: now, Request: engine.Request{Kind: "execute"}},
		{ID: "old", Node: "one", Status: "failed", Created: now.Add(-25 * time.Hour), Request: engine.Request{Kind: "execute"}},
	} {
		if err = db.CreateTask(job); err != nil {
			t.Fatal(err)
		}
	}
	c := Settings{Enabled: true}
	s.CheckCleanup(context.Background(), c, now)
	events, _ := s.Events()
	if len(events) != 0 {
		t.Fatal("not opt in")
	}
	c.CleanupFailures = true
	c.Enabled = false
	s.CheckCleanup(context.Background(), c, now)
	events, _ = s.Events()
	if len(events) != 0 {
		t.Fatal("globally disabled")
	}
	c.Enabled = true
	if err = s.CheckCleanup(context.Background(), c, now); err != nil {
		t.Fatal(err)
	}
	events, err = s.Events()
	if err != nil || len(events) != 1 || events[0].Kind != "cleanup_failure" || events[0].Task != "failed" {
		t.Fatal(events, err)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "/private") {
		t.Fatal("failure details leaked")
	}
	s, err = New(db.DB, "")
	if err != nil {
		t.Fatal(err)
	}
	s.CheckCleanup(context.Background(), c, now.Add(time.Minute))
	events, _ = s.Events()
	if len(events) != 1 {
		t.Fatal("restart replayed", events)
	}
	// The reused webhook blocks loopback and records failure without raw URL/error.
	db.CreateTask(engine.Task{ID: "interrupted", Node: "one", Status: "interrupted", Created: now, Request: engine.Request{Kind: "execute"}})
	s.Webhook = "https://127.0.0.1/secret"
	if err = s.CheckCleanup(context.Background(), c, now); err != nil {
		t.Fatal(err)
	}
	events, _ = s.Events()
	if len(events) != 2 || events[0].Delivery != "failed" {
		t.Fatal(events)
	}
	s.CheckCleanup(context.Background(), c, now)
	events, _ = s.Events()
	if len(events) != 2 {
		t.Fatal("failed delivery replayed")
	}
}
