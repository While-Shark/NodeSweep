package hub

import (
	"bytes"
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
	if err = s.SaveTask(task); err != nil {
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
