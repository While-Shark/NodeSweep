package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/While-Shark/NodeSweep/internal/engine"
)

func TestPollURLRejectsSecretBearingAndInsecureURLs(t *testing.T) {
	for _, input := range []string{"http://example.com", "https:///missing", "https://user:secret@example.com", "https://example.com?token=secret", "https://example.com/#secret", "https://example.com/%2fother"} {
		if _, err := pollURL(input); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	for _, input := range []string{"https://example.com/panel/", "http://127.0.0.1:9780", "http://[::1]:9780"} {
		got, err := pollURL(input)
		if err != nil || !strings.HasSuffix(got, "/agent/poll") {
			t.Fatal(got, err)
		}
	}
}
func TestRedirectNeverForwardsNodeCredential(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	req, _ := http.NewRequest("POST", source.URL, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 64))
	res, err := agentClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if reached || res.StatusCode != 307 {
		t.Fatal("followed redirect with credentials")
	}
}
func TestReadTaskRejectsCrossNodeShellPayloadAndTrailingJSON(t *testing.T) {
	task := engine.Task{ID: engine.ID(), Node: "node-a", Status: "running", Request: engine.Request{Kind: "detect"}}
	encode := func(t engine.Task) string { b, _ := json.Marshal(map[string]any{"task": t}); return string(b) }
	if _, err := readTask(strings.NewReader(encode(task)), "node-a"); err != nil {
		t.Fatal(err)
	}
	other := task
	other.Node = "node-b"
	shell := task
	shell.Request.Kind = "shell"
	for _, data := range []string{encode(other), encode(shell), encode(task) + `{}`, `{"task":{"id":"` + task.ID + `","node":"node-a","status":"running","request":{"kind":"detect","command":"bash -c anything"}}}`, strings.Repeat("x", (1<<20)+1)} {
		if _, err := readTask(strings.NewReader(data), "node-a"); err == nil {
			t.Fatal("accepted malformed or executable task")
		}
	}
	if _, err := readTask(io.LimitReader(strings.NewReader(`{"task":null}`), 13), "node-a"); err != nil {
		t.Fatal(err)
	}
}
