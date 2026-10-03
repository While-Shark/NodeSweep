package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/hub"
	"github.com/While-Shark/NodeSweep/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestOutboundAgentScanCancellation(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10000; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%05d.log", i)), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	node := engine.ID()
	token := strings.Repeat("n", 64)
	if err = s.AddNode(store.Node{ID: node, Name: "agent", LastSeen: time.Now()}, hub.Hash(token)); err != nil {
		t.Fatal(err)
	}
	h := &hub.Hub{Store: s, Token: strings.Repeat("a", 64)}
	server := httptest.NewServer(h.Handler(fstest.MapFS{}))
	defer server.Close()
	job := engine.Task{ID: engine.ID(), Node: node, Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "scan", Path: root}}
	if err = s.CreateTask(job); err != nil {
		t.Fatal(err)
	}
	e := engine.New(nil, []string{root})
	e.ScanBudget = engine.ScanBudget{PauseMillis: 100, TreeBytes: 16 << 20}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, server.URL, node, token, e) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(18 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("scan state timed out")
	}
	wait(func() bool { current, _ := s.Task(job.ID); return current.Status == "running" })
	req, _ := http.NewRequest("POST", server.URL+"/api/tasks/"+job.ID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+h.Token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	wait(func() bool { current, _ := s.Task(job.ID); return current.Status == "failed" })
	final, err := s.Task(job.ID)
	if err != nil || !final.CancelRequested || !strings.Contains(final.Error, "context canceled") {
		t.Fatal(final, err)
	}
	raw, _ := json.Marshal(final.Result)
	var partial engine.Scan
	if err = json.Unmarshal(raw, &partial); err != nil || partial.Tree == nil || !partial.Truncated || partial.Reason != "cancelled" || partial.Files >= 10000 || partial.Files == 0 {
		t.Fatal("partial result missing", partial.Files, partial.Reason, err)
	}
	if final.Progress == nil || final.Progress.Visited < 1 {
		t.Fatal("progress missing")
	}
}
func TestCancellationResponseValidation(t *testing.T) {
	id := engine.ID()
	if _, err := readResponse(strings.NewReader(`{"task":null,"cancel":"`+id+`"}`), "node"); err != nil {
		t.Fatal(err)
	}
	if _, err := readResponse(strings.NewReader(`{"task":null,"cancel":"not-an-id"}`), "node"); err == nil {
		t.Fatal("invalid cancellation accepted")
	}
}
