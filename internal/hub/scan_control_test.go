package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestScanCancelGuardsAndLocalContext(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	root := t.TempDir()
	e := engine.New(nil, []string{root})
	h := &Hub{Store: s, Engine: e, Token: strings.Repeat("a", 64), Context: context.Background()}
	s.AddNode(store.Node{ID: "local", Name: "local", LastSeen: time.Now(), ScanControl: true}, "")
	s.AddNode(store.Node{ID: "old", Name: "old", LastSeen: time.Now()}, Hash("agent"))
	handler := h.Handler(fstest.MapFS{})
	call := func(id, token string) int {
		request := httptest.NewRequest("POST", "/api/tasks/"+id+"/cancel", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Code
	}
	for _, job := range []engine.Task{{ID: "delete", Node: "local", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "execute"}}, {ID: "oldscan", Node: "old", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "scan", Path: root}}} {
		if err = s.CreateTask(job); err != nil {
			t.Fatal(err)
		}
	}
	if call("delete", h.Token) != 409 || call("oldscan", h.Token) != 409 || call("oldscan", "agent") != 401 || call("missing", h.Token) != 404 {
		t.Fatal("cancellation guard failed")
	}
	job := engine.Task{ID: engine.ID(), Node: "local", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "scan", Path: root}}
	if err = s.CreateTask(job); err != nil {
		t.Fatal(err)
	}
	// A pending scan is removed from the queue atomically, before it can acquire a worker.
	if call(job.ID, h.Token) != 200 {
		t.Fatal("pending scan cancel failed")
	}
	if claimed, err := s.Claim(job.ID, "local"); err != nil || claimed != nil {
		t.Fatal("pending cancellation replayed", err)
	}
	// Cancellation callbacks are scoped to a matching scan; no execute callback is invoked.
	invoked := false
	h.cancels = map[string]context.CancelFunc{"delete": func() { invoked = true }}
	call("delete", h.Token)
	if invoked {
		t.Fatal("execute context cancelled")
	}
	progress := Poll{Node: "old", Busy: true, TaskID: engine.ID(), Progress: &engine.ScanProgress{Visited: 2, Files: 3, Limit: 100}}
	if validatePoll(progress) == nil {
		t.Fatal("invalid progress accepted")
	}
	raw, _ := json.Marshal(progress)
	if len(raw) == 0 {
		t.Fatal("empty metadata")
	}
}

func TestCancelRunningStandaloneScan(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	root := t.TempDir()
	for i := 0; i < 1500; i++ {
		if err = os.WriteFile(filepath.Join(root, fmt.Sprint(i)), []byte("log"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e := engine.New(nil, []string{root})
	e.ScanBudget = engine.ScanBudget{PauseMillis: 100}
	h := &Hub{Store: s, Engine: e, Token: strings.Repeat("a", 64), Context: context.Background()}
	s.AddNode(store.Node{ID: "local", LastSeen: time.Now(), ScanControl: true}, "")
	job := engine.Task{ID: engine.ID(), Node: "local", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "scan", Path: root}}
	if err = s.CreateTask(job); err != nil {
		t.Fatal(err)
	}
	h.workers.Add(1)
	go h.localTask(job)
	deadline := time.Now().Add(2 * time.Second)
	for {
		task, err := s.Task(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Progress != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no progress")
		}
		time.Sleep(time.Millisecond)
	}
	req := httptest.NewRequest("POST", "/api/tasks/"+job.ID+"/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+h.Token)
	res := httptest.NewRecorder()
	h.Handler(fstest.MapFS{}).ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	h.workers.Wait()
	task, err := s.Task(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var partial engine.Scan
	raw, _ := json.Marshal(task.Result)
	json.Unmarshal(raw, &partial)
	if task.Status != "failed" || !task.CancelRequested || partial.Reason != "cancelled" || partial.Tree == nil {
		t.Fatal(task, partial)
	}
}
