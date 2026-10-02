package hub

import (
	"encoding/json"
	"github.com/While-Shark/NodeSweep/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNodeMetadataAuthorizationAndValidation(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	h := &Hub{Store: s, Token: strings.Repeat("a", 32)}
	if err = s.AddNode(store.Node{ID: "node", Name: "original"}, Hash("agent-token")); err != nil {
		t.Fatal(err)
	}
	handler := h.Handler(fstest.MapFS{})
	call := func(method, path, token, body string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if code := call("PATCH", "/api/nodes/node", "agent-token", `{"name":"bad","group":"bad"}`); code != 401 {
		t.Fatal(code)
	}
	for _, body := range []string{`{"name":"","group":"x"}`, `{"name":"x","group":"a\nb"}`, `{"name":"x","group":"x","token":"leak"}`} {
		if code := call("PATCH", "/api/nodes/node", h.Token, body); code != 400 {
			t.Fatal(code, body)
		}
	}
	body, _ := json.Marshal(map[string]string{"name": "東京节点", "group": strings.Repeat("韩", 64)})
	if code := call("PATCH", "/api/nodes/node", h.Token, string(body)); code != 200 {
		t.Fatal(code)
	}
	n, err := s.Node("node")
	if err != nil || n.Name != "東京节点" || n.Group != strings.Repeat("韩", 64) {
		t.Fatal(n, err)
	}
	if code := call("PATCH", "/api/nodes/missing", h.Token, `{"name":"x"}`); code != 404 {
		t.Fatal(code)
	}
	if code := call("POST", "/api/nodes", h.Token, `{"name":"new","group":"test"}`); code != 200 {
		t.Fatal(code)
	}
	if validMetadata("name", strings.Repeat("x", 65)) {
		t.Fatal("overlong group accepted")
	}
}
