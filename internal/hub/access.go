package hub

import (
	"errors"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
	"log"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

type AccessToken struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	Token string `json:"token"`
}

func ValidateAccess(admin string, credentials []AccessToken) error {
	if len(credentials) > 32 {
		return errors.New("at most 32 access tokens allowed")
	}
	seen := map[string]bool{Hash(admin): true}
	names := map[string]bool{"admin": true}
	for _, c := range credentials {
		if c.Role != "viewer" && c.Role != "operator" && c.Role != "admin" {
			return errors.New("invalid access role")
		}
		if !utf8.ValidString(c.Name) || c.Name == "" || len(c.Name) > 64 || strings.TrimSpace(c.Name) != c.Name || strings.ContainsFunc(c.Name, unicode.IsControl) || names[c.Name] {
			return errors.New("invalid or duplicate access name")
		}
		if len(c.Token) < 32 || len(c.Token) > 256 || bearer("Bearer "+c.Token) != c.Token || seen[Hash(c.Token)] {
			return errors.New("invalid or duplicate access token")
		}
		seen[Hash(c.Token)] = true
		names[c.Name] = true
	}
	return nil
}
func (h *Hub) principal(credential string) (name, role string) {
	if credential == "" {
		return "", ""
	}
	hash := Hash(credential)
	if len(h.Token) >= 32 && equal(hash, Hash(h.Token)) {
		name, role = "admin", "admin"
	}
	for _, c := range h.AccessTokens {
		if equal(hash, Hash(c.Token)) {
			name, role = c.Name, c.Role
		}
	}
	return
}
func permitted(role, p, method string) bool {
	if role == "admin" {
		return true
	}
	if role != "viewer" && role != "operator" {
		return false
	}
	if method == "GET" {
		return p != "audit"
	}
	if role == "operator" && method == "POST" {
		return p == "tasks" || strings.HasPrefix(p, "tasks/") && strings.HasSuffix(p, "/cancel")
	}
	return false
}

// Record route categories only. User-controlled URLs, query strings and request
// bodies can contain secrets and must not enter the audit trail.
func auditAction(p, method string) (action, target string) {
	pieces := strings.Split(p, "/")
	resource := pieces[0]
	switch resource {
	case "nodes", "rules", "tasks", "alerts":
	default:
		resource = "unknown"
	}
	action = resource + "." + strings.ToLower(method)
	if len(pieces) > 1 && (pieces[1] == "local" || engine.ValidID(pieces[1])) {
		target = pieces[1]
	}
	if resource == "tasks" && len(pieces) == 3 && pieces[2] == "cancel" {
		action = "tasks.cancel"
	}
	return
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(b)
}
func (h *Hub) authenticatedAPI(w http.ResponseWriter, r *http.Request) {
	name, role := h.principal(bearer(r.Header.Get("Authorization")))
	if role == "" {
		fail(w, errors.New("invalid administrator token"), 401)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/")
	allowed := permitted(role, p, r.Method)
	var auditID int64
	if r.Method != "GET" || !allowed {
		action, target := auditAction(p, r.Method)
		status := 0
		if !allowed {
			status = 403
		}
		var err error
		auditID, err = h.Store.BeginAudit(store.AuditEvent{Actor: name, Role: role, Action: action, Target: target, Status: status})
		if err != nil {
			fail(w, err, 500)
			return
		}
	}
	if !allowed {
		fail(w, errors.New("insufficient role permissions"), 403)
		return
	}
	if p == "session" && r.Method == "GET" {
		reply(w, map[string]string{"name": name, "role": role})
		return
	}
	recorder := &statusWriter{ResponseWriter: w}
	h.api(recorder, r)
	if auditID != 0 {
		status := recorder.status
		if status == 0 {
			status = 200
		}
		if err := h.Store.FinishAudit(auditID, status); err != nil {
			log.Print("audit outcome could not be persisted")
		}
	}
}
