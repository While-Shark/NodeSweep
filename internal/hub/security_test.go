package hub

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
)

func TestNodeCannotObserveOrCompleteAnotherNodesTask(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	tokens := map[string]string{"node-a": strings.Repeat("a", 64), "node-b": strings.Repeat("b", 64)}
	for id, token := range tokens {
		if err := s.AddNode(store.Node{ID: id, Name: id}, Hash(token)); err != nil {
			t.Fatal(err)
		}
	}
	task := engine.Task{ID: engine.ID(), Node: "node-b", Status: "pending", Created: time.Now(), Request: engine.Request{Kind: "detect"}}
	if err := s.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	h := &Hub{Store: s, Token: strings.Repeat("c", 64)}
	handler := h.Handler(fstest.MapFS{"index.html": {Data: []byte("ok")}})
	poll := func(body Poll) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/agent/poll", bytes.NewReader(raw))
		req.Header.Set("X-Node-ID", "node-a")
		req.Header.Set("Authorization", "Bearer "+tokens["node-a"])
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	if res := poll(Poll{Node: "node-a"}); res.Code != 200 || strings.Contains(res.Body.String(), task.ID) {
		t.Fatal("cross-node task disclosure", res.Code, res.Body.String())
	}
	if res := poll(Poll{Node: "node-b"}); res.Code != 401 {
		t.Fatal("spoofed node accepted")
	}
	if _, err := s.Claim(task.ID, "node-b"); err != nil {
		t.Fatal(err)
	}
	forged := task
	forged.Status = "succeeded"
	forged.Result = "forged"
	forged.Node = "node-b"
	if res := poll(Poll{Node: "node-a", Result: &forged}); res.Code != 400 {
		t.Fatal("cross-node completion accepted", res.Code)
	}
	got, _ := s.Task(task.ID)
	if got.Status != "running" {
		t.Fatal("changed another node's task")
	}
	bad := Poll{Node: "node-a", Metrics: engine.Metrics{Host: strings.Repeat("x", 10000)}}
	if res := poll(bad); res.Code != 400 {
		t.Fatal("accepted oversized metadata")
	}
	n, _ := s.Node("node-a")
	if n.Metrics.Host != "" {
		t.Fatal("stored rejected metadata")
	}
}
func TestBodyBudgetBearerAndInternalErrorRedaction(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	h := &Hub{Store: s, Token: strings.Repeat("a", 64)}
	handler := h.Handler(fstest.MapFS{"index.html": {Data: []byte("ok")}})
	call := func(method, path, auth, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if res := call("GET", "/api/nodes", h.Token, ""); res.Code != 401 {
		t.Fatal("accepted non-Bearer credential")
	}
	if res := call("POST", "/api/nodes", "Bearer "+h.Token, `{"name":"`+strings.Repeat("x", adminBodyLimit)+`"}`); res.Code != 400 {
		t.Fatal("accepted oversized admin body")
	}
	if res := call("POST", "/api/tasks", "Bearer "+h.Token, `{"node":"local","request":{"kind":"shell","command":"ignored"}}`); res.Code != 400 {
		t.Fatal("accepted command field")
	}
	s.DB.Close()
	if res := call("GET", "/api/nodes", "Bearer "+h.Token, ""); res.Code != 500 || strings.Contains(res.Body.String(), "database") {
		t.Fatal("disclosed database error", res.Body.String())
	}
	empty := (&Hub{Store: s}).Handler(fstest.MapFS{})
	req := httptest.NewRequest("GET", "/api/nodes", nil)
	res := httptest.NewRecorder()
	empty.ServeHTTP(res, req)
	if res.Code != 401 {
		t.Fatal("empty admin token allowed access")
	}
}
func TestPollGateBoundsConcurrencyAndBurst(t *testing.T) {
	gate := pollGate{}
	release, ok := gate.enter("node")
	if !ok {
		t.Fatal("first poll rejected")
	}
	if _, ok := gate.enter("node"); ok {
		t.Fatal("parallel poll allowed")
	}
	release()
	for i := 0; i < 5; i++ {
		release, ok = gate.enter("node")
		if !ok {
			t.Fatal("expected burst")
		}
		release()
	}
	if _, ok := gate.enter("node"); ok {
		t.Fatal("unbounded poll burst")
	}
	if release, ok := gate.enter("other"); !ok {
		t.Fatal("one node blocked another")
	} else {
		release()
	}
}
