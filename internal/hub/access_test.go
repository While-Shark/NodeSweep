package hub

import (
	"encoding/json"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestRolesEnforceServerAuthorizationAndRedactedAudit(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	h := &Hub{Store: s, Token: strings.Repeat("a", 64), AccessTokens: []AccessToken{{"observer", "viewer", strings.Repeat("v", 64)}, {"runner", "operator", strings.Repeat("o", 64)}, {"second-admin", "admin", strings.Repeat("b", 64)}}}
	node := store.Node{ID: engine.ID(), LastSeen: time.Now(), ScanControl: true}
	s.AddNode(node, Hash("agent-token"))
	handler := h.Handler(fstest.MapFS{})
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/"+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	viewer, operator := h.AccessTokens[0].Token, h.AccessTokens[1].Token
	if call("GET", "nodes", viewer, "").Code != 200 || call("GET", "session", operator, "").Code != 200 || call("GET", "nodes", "agent-token", "").Code != 401 {
		t.Fatal("credential isolation failed")
	}
	body := `{"node":"` + node.ID + `","request":{"kind":"execute","planId":"` + engine.ID() + `"}}`
	if call("POST", "tasks", viewer, body).Code != 403 || call("POST", "nodes", operator, `{"name":"secret-body"}`).Code != 403 || call("GET", "audit", operator, "").Code != 403 {
		t.Fatal("role bypass")
	}
	if w := call("POST", "tasks", operator, body); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if call("POST", "nodes", h.AccessTokens[2].Token, `{"name":"second VPS"}`).Code != 200 {
		t.Fatal("additional admin rejected")
	}
	call("DELETE", "unknown/raw-secret?token=secret-query", viewer, "secret-body")
	response := call("GET", "audit", h.Token, "")
	var events []store.AuditEvent
	json.Unmarshal(response.Body.Bytes(), &events)
	if response.Code != 200 || len(events) != 6 {
		t.Fatal(response.Code, len(events), response.Body.String())
	}
	raw := response.Body.String()
	if strings.Contains(raw, viewer) || strings.Contains(raw, operator) || strings.Contains(raw, "secret-") || strings.Contains(raw, "agent-token") {
		t.Fatal("audit leaked secret")
	}
	found := false
	for _, e := range events {
		if e.Actor == "runner" && e.Action == "tasks.post" && e.Status == 200 {
			found = true
		}
	}
	if !found {
		t.Fatal("operator task request not audited")
	}
	// Fail closed before mutation if the durable audit trail is unavailable.
	s.DB.Exec("DROP TABLE audit_events")
	if call("POST", "nodes", h.Token, `{"name":"must not exist"}`).Code != 500 {
		t.Fatal("failed open")
	}
	nodes, _ := s.Nodes()
	if len(nodes) != 2 {
		t.Fatal("unaudited mutation", nodes)
	}
}
func TestAccessConfigRejectsWeakDuplicateOrUnknownCredentials(t *testing.T) {
	admin := strings.Repeat("a", 64)
	for _, c := range []AccessToken{{"ok", "root", strings.Repeat("b", 64)}, {"ok", "viewer", admin}, {"ok", "viewer", "weak"}, {"ok", "viewer", strings.Repeat("b", 32) + " "}, {"admin", "operator", strings.Repeat("b", 64)}, {"bad\nname", "viewer", strings.Repeat("b", 64)}} {
		if ValidateAccess(admin, []AccessToken{c}) == nil {
			t.Fatal("invalid credential accepted", c.Name, c.Role)
		}
	}
	if err := ValidateAccess(admin, []AccessToken{{"ok", "viewer", strings.Repeat("b", 64)}}); err != nil {
		t.Fatal(err)
	}
}
