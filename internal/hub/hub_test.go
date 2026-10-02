package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestAuthenticationAndNodeIsolation(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	h := &Hub{Store: s, Token: strings.Repeat("a", 64)}
	server := httptest.NewServer(h.Handler(fstest.MapFS{"index.html": {Data: []byte("hello")}}))
	defer server.Close()
	request := func(method, path, token string, b any) (int, []byte) {
		t.Helper()
		raw, _ := json.Marshal(b)
		r, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		if poll, ok := b.(Poll); ok {
			r.Header.Set("X-Node-ID", poll.Node)
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, data
	}
	if code, _ := request("GET", "/api/nodes", "", nil); code != 401 {
		t.Fatal(code)
	}
	code, data := request("POST", "/api/nodes", h.Token, map[string]string{"name": "remote"})
	if code != 200 {
		t.Fatal(code, string(data))
	}
	var created struct {
		Node  store.Node `json:"node"`
		Token string     `json:"token"`
	}
	if err = json.Unmarshal(data, &created); err != nil {
		t.Fatal(err)
	}
	if code, _ = request("GET", "/api/nodes", created.Token, nil); code != 401 {
		t.Fatal("node token accessed admin API")
	}
	if code, _ = request("POST", "/agent/poll", h.Token, Poll{Node: created.Node.ID}); code != 401 {
		t.Fatal("admin token accessed node API")
	}
	if code, data = request("POST", "/agent/poll", created.Token, Poll{Node: created.Node.ID}); code != 200 {
		t.Fatal(code, string(data))
	}
	task := engine.Task{ID: engine.ID(), Node: created.Node.ID, Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "detect"}}
	if err = s.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	code, data = request("POST", "/agent/poll", created.Token, Poll{Node: created.Node.ID})
	if code != 200 || !bytes.Contains(data, []byte(task.ID)) {
		t.Fatal(code, string(data))
	}
	task.Status = "succeeded"
	task.Result = []string{"ok"}
	if code, data = request("POST", "/agent/poll", created.Token, Poll{Node: created.Node.ID, Result: &task}); code != 200 {
		t.Fatal(code, string(data))
	}
	task.Node = "other"
	task.ID = "unknown"
	if code, _ = request("POST", "/agent/poll", created.Token, Poll{Node: created.Node.ID, Result: &task}); code != 400 {
		t.Fatal("accepted unknown task")
	}
	if code, _ = request("DELETE", "/api/nodes/"+created.Node.ID, h.Token, nil); code != 200 {
		t.Fatal(code)
	}
	if code, _ = request("POST", "/agent/poll", created.Token, Poll{Node: created.Node.ID}); code != 401 {
		t.Fatal("revoked token accepted")
	}
}

func TestRuleTransferEndpoints(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	h := &Hub{Store: s, Token: strings.Repeat("b", 64)}
	handler := h.Handler(fstest.MapFS{"index.html": {Data: []byte("hello")}})
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	r := engine.Rule{ID: "from-file", Name: "history", Scheme: "shared", Root: "/var/log/app", KeepDays: 14, Patterns: []string{"*.log.*"}}
	b := engine.RuleBundle{Format: engine.BundleFormat, Version: engine.BundleVersion, Rules: []engine.Rule{r}}
	if res := call("POST", "/api/rules/import", "invalid", b); res.Code != 401 {
		t.Fatal(res.Code)
	}
	if res := call("POST", "/api/rules/import", h.Token, b); res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	res := call("GET", "/api/rules/export", h.Token, nil)
	if res.Code != 200 {
		t.Fatal(res.Code)
	}
	var exported engine.RuleBundle
	if err = json.Unmarshal(res.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Format != engine.BundleFormat || len(exported.Rules) != 1 || exported.Rules[0].ID != "" {
		t.Fatal(exported)
	}
	if res = call("POST", "/api/rules/import", h.Token, exported); res.Code != 200 || !strings.Contains(res.Body.String(), `"skipped":1`) {
		t.Fatal(res.Code, res.Body.String())
	}
	before, _ := s.Rules()
	b.Rules[0].Name = strings.Repeat("x", 300000)
	if res = call("POST", "/api/rules/import", h.Token, b); res.Code != 400 {
		t.Fatal("oversized bundle accepted", res.Code)
	}
	after, _ := s.Rules()
	if len(before) != len(after) {
		t.Fatal("oversized bundle changed rules")
	}
}

func TestShutdownDrainsCancelledLocalTask(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := &Hub{Store: s, Token: strings.Repeat("c", 64), Context: ctx, Engine: engine.New(nil, []string{root})}
	task := engine.Task{ID: "local-work", Node: "local", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "scan", Path: root}}
	if err = s.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	h.workers.Add(1)
	go h.localTask(task)
	h.Wait()
	got, err := s.Task(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || !strings.Contains(got.Error, "context canceled") {
		t.Fatalf("did not drain task: %+v", got)
	}
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"node":"local","request":{"kind":"detect"}}`))
	req.Header.Set("Authorization", "Bearer "+h.Token)
	res := httptest.NewRecorder()
	h.Handler(fstest.MapFS{"index.html": {Data: []byte("hello")}}).ServeHTTP(res, req)
	if res.Code != 503 {
		t.Fatal("accepted new task during shutdown", res.Code)
	}
}
