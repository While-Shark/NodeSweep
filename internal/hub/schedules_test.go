package hub

import (
	"bytes"
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

func TestScheduledConsentOwnershipAndSnapshot(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	now := time.Now()
	node := engine.ID()
	other := engine.ID()
	for _, id := range []string{node, other} {
		if err = s.AddNode(store.Node{ID: id, LastSeen: now}, "key"); err != nil {
			t.Fatal(err)
		}
	}
	rule := engine.Rule{ID: engine.ID(), Name: "archives", Root: "/var/log", KeepDays: 14, Patterns: []string{"*.log.*"}}
	if err = s.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	h := &Hub{Store: s, Token: strings.Repeat("a", 64), AccessTokens: []AccessToken{{Name: "operator", Role: "operator", Token: strings.Repeat("o", 64)}}}
	handler := h.Handler(fstest.MapFS{"index.html": {Data: []byte("ok")}})
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	res := call("POST", "/api/schedules", h.Token, map[string]any{"node": node, "ruleId": rule.ID, "hours": 1})
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	var schedule store.Schedule
	if err = json.Unmarshal(res.Body.Bytes(), &schedule); err != nil {
		t.Fatal(err)
	}
	if schedule.Enabled {
		t.Fatal("enabled by default")
	}
	path := "/api/schedules/" + schedule.ID
	if res = call("PATCH", path, strings.Repeat("o", 64), map[string]any{"enabled": true, "confirm": true}); res.Code != 403 {
		t.Fatal("operator enabled unattended cleanup", res.Code)
	}
	if res = call("PATCH", path, h.Token, map[string]any{"enabled": true, "confirm": true}); res.Code != 409 {
		t.Fatal("missing preview accepted", res.Code)
	}
	preview := func(id string, rule engine.Rule) engine.Task {
		task := engine.Task{ID: engine.ID(), Node: id, Status: "succeeded", Created: now, Request: engine.Request{Kind: "preview", Rule: rule}, Result: engine.Plan{ID: engine.ID(), Created: now, Rule: rule, Files: []engine.Candidate{{Path: "/var/log/x.log.1"}}}}
		if err = s.CreateTask(task); err != nil {
			t.Fatal(err)
		}
		return task
	}
	foreign := preview(other, rule)
	if res = call("PATCH", path, h.Token, map[string]any{"enabled": true, "confirm": true, "previewTask": foreign.ID}); res.Code != 409 {
		t.Fatal("cross-node consent", res.Code)
	}
	own := preview(node, rule)
	if res = call("PATCH", path, h.Token, map[string]any{"enabled": true, "previewTask": own.ID}); res.Code != 409 {
		t.Fatal("missing confirmation", res.Code)
	}
	if res = call("PATCH", path, h.Token, map[string]any{"enabled": true, "confirm": true, "previewTask": own.ID}); res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	rule.Root = "/var/log/changed"
	if err = s.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Schedule(schedule.ID)
	if err != nil || saved.Rule.Root != "/var/log" || !saved.Enabled || saved.Next.Before(now.Add(59*time.Minute)) {
		t.Fatal("snapshot/next changed", saved, err)
	}
	if res = call("PATCH", path, h.Token, map[string]any{"enabled": false}); res.Code != 200 {
		t.Fatal(res.Code)
	}
}

func TestScheduledPipelineNoReplayOverlapAndAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.DB.Close() }()
	now := time.Now()
	node := engine.ID()
	if err = s.AddNode(store.Node{ID: node, LastSeen: now}, "key"); err != nil {
		t.Fatal(err)
	}
	rule := engine.Rule{ID: engine.ID(), Name: "archives", Root: "/var/log", KeepDays: 14, Patterns: []string{"*.log.*"}}
	v := store.Schedule{ID: engine.ID(), Node: node, Rule: rule, Hours: 1, Enabled: true, Next: now.Add(-time.Second)}
	if err = s.SaveSchedule(v); err != nil {
		t.Fatal(err)
	}
	h := &Hub{Store: s}
	if err = h.scheduleTick(now); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Schedule(v.ID)
	if v.Phase != "preview" || !v.Next.After(now) {
		t.Fatal(v)
	}
	first := v.Task
	if err = h.scheduleTick(now); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Schedule(v.ID)
	if v.Task != first {
		t.Fatal("overlap")
	}
	task, err := s.Next(node)
	if err != nil || task == nil {
		t.Fatal(err)
	}
	plan := engine.Plan{ID: engine.ID(), Created: now, Rule: rule, Files: []engine.Candidate{{Path: "/var/log/x.log.1"}}}
	task.Status = "succeeded"
	task.Result = plan
	if err = s.Complete(*task); err != nil {
		t.Fatal(err)
	}
	if err = h.scheduleTick(now); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Schedule(v.ID)
	execute, err := s.Task(v.Task)
	if err != nil || v.Phase != "execute" || execute.Request.PlanID != plan.ID || execute.Node != node {
		t.Fatal(v, execute, err)
	}
	audit, err := s.Audit()
	if err != nil || len(audit) != 2 || audit[0].Action != "schedules.execute" {
		t.Fatal(audit, err)
	}
	if err = s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h.Store = s
	v, _ = s.Schedule(v.ID)
	if v.Enabled || v.Phase != "" || v.Outcome != "restart_review_required" {
		t.Fatal("restart replay", v)
	}
	if err = h.scheduleTick(now.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	tasks, err := s.Tasks()
	if err != nil || len(tasks) != 2 {
		t.Fatal("replayed after restart", tasks, err)
	}
	// Audit unavailable must block task dispatch.
	v.Enabled = true
	v.Next = now
	v.Task = ""
	if err = s.SaveSchedule(v); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("DROP TABLE audit_events"); err != nil {
		t.Fatal(err)
	}
	if err = h.scheduleTick(now); err == nil {
		t.Fatal("missing audit accepted")
	}
	tasks, _ = s.Tasks()
	if len(tasks) != 2 {
		t.Fatal("unaudited task created")
	}
}

func TestScheduleSkipsBusyAndPausesFailedPreview(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	now := time.Now()
	node := engine.ID()
	s.AddNode(store.Node{ID: node, LastSeen: now}, "key")
	v := store.Schedule{ID: engine.ID(), Node: node, Hours: 1, Enabled: true, Next: now}
	s.SaveSchedule(v)
	pending := engine.Task{ID: engine.ID(), Node: node, Status: "pending", Created: now, Request: engine.Request{Kind: "detect"}}
	s.CreateTask(pending)
	h := &Hub{Store: s}
	if err = h.scheduleTick(now); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Schedule(v.ID)
	if v.Outcome != "busy" || !v.Next.After(now) || v.Phase != "" {
		t.Fatal(v)
	}
	pending.Status = "failed"
	pending.Result = nil
	if _, err = s.Claim(pending.ID, node); err != nil {
		t.Fatal(err)
	}
	if err = s.Complete(pending); err != nil {
		t.Fatal(err)
	}
	v.Task = pending.ID
	v.Phase = "preview"
	s.SaveSchedule(v)
	if err = h.scheduleTick(now); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Schedule(v.ID)
	if v.Enabled || v.Phase != "" || v.Outcome != "failed" {
		t.Fatal(v)
	}
}

func TestSchedulesBoundPipelinesAndStopBetweenPreviewAndExecute(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	now := time.Now()
	for i := 0; i < 3; i++ {
		node := engine.ID()
		if err = s.AddNode(store.Node{ID: node, LastSeen: now}, "key"); err != nil {
			t.Fatal(err)
		}
		v := store.Schedule{ID: engine.ID(), Node: node, Hours: 1, Enabled: true, Next: now}
		if err = s.SaveSchedule(v); err != nil {
			t.Fatal(err)
		}
	}
	h := &Hub{Store: s, Token: strings.Repeat("a", 64)}
	if err = h.scheduleTick(now); err != nil {
		t.Fatal(err)
	}
	list, err := s.Schedules()
	if err != nil {
		t.Fatal(err)
	}
	active, skipped := 0, 0
	var selected store.Schedule
	for _, v := range list {
		if v.Phase == "preview" {
			active++
			selected = v
		} else if v.Outcome == "busy" {
			skipped++
		}
	}
	if active != 2 || skipped != 1 {
		t.Fatal(active, skipped)
	}
	req := httptest.NewRequest("PATCH", "/api/schedules/"+selected.ID, strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Authorization", "Bearer "+h.Token)
	res := httptest.NewRecorder()
	h.Handler(fstest.MapFS{}).ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	task, err := s.Claim(selected.Task, selected.Node)
	if err != nil || task == nil {
		t.Fatal(err)
	}
	task.Status = "succeeded"
	task.Result = engine.Plan{ID: engine.ID(), Created: now, Rule: selected.Rule, Files: []engine.Candidate{{Path: "old.log.1"}}}
	if err = s.Complete(*task); err != nil {
		t.Fatal(err)
	}
	if err = h.scheduleTick(now); err != nil {
		t.Fatal(err)
	}
	tasks, err := s.Tasks()
	if err != nil || len(tasks) != 2 {
		t.Fatal("execution submitted after pause", tasks, err)
	}
	saved, _ := s.Schedule(selected.ID)
	if saved.Enabled || saved.Phase != "" {
		t.Fatal(saved)
	}
}
