package hub

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"
)

func previewPlan(t engine.Task, node string, rule engine.Rule, now time.Time) (engine.Plan, error) {
	var plan engine.Plan
	if t.Node != node || t.Status != "succeeded" || t.Request.Kind != "preview" || !reflect.DeepEqual(t.Request.Rule, rule) {
		return plan, errors.New("fresh matching preview required")
	}
	raw, err := json.Marshal(t.Result)
	if err != nil {
		return plan, err
	}
	if err = json.Unmarshal(raw, &plan); err != nil {
		return plan, err
	}
	if !engine.ValidID(plan.ID) || !reflect.DeepEqual(plan.Rule, rule) || plan.Created.After(now.Add(30*time.Second)) || now.Sub(plan.Created) > 5*time.Minute || now.Sub(t.Created) > 5*time.Minute || t.Created.After(now.Add(30*time.Second)) {
		return plan, errors.New("fresh matching preview required")
	}
	return plan, nil
}

func (h *Hub) schedulesAPI(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.Method == "GET" && r.URL.Path == "/api/schedules" {
		v, err := h.Store.Schedules()
		if err != nil {
			fail(w, err, 500)
			return
		}
		reply(w, v)
		return
	}
	if h.stopping || h.workContext().Err() != nil {
		fail(w, errors.New("server shutting down"), 503)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/api/schedules" {
		var b struct {
			Node   string `json:"node"`
			RuleID string `json:"ruleId"`
			Hours  int    `json:"hours"`
		}
		if err := decode(w, r, &b); err != nil {
			fail(w, err, 400)
			return
		}
		if b.Hours < 1 || b.Hours > 720 {
			fail(w, errors.New("schedule interval must be 1–720 hours"), 400)
			return
		}
		if _, err := h.Store.Node(b.Node); err != nil {
			fail(w, errors.New("node not found"), 404)
			return
		}
		rules, err := h.Store.Rules()
		if err != nil {
			fail(w, err, 500)
			return
		}
		for _, rule := range rules {
			if rule.ID == b.RuleID {
				v := store.Schedule{ID: engine.ID(), Node: b.Node, Rule: rule, Hours: b.Hours, Outcome: "paused"}
				if err = h.Store.SaveSchedule(v); err != nil {
					fail(w, errors.New("schedule limit reached or storage unavailable"), 409)
					return
				}
				reply(w, v)
				return
			}
		}
		fail(w, errors.New("rule not found"), 404)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/schedules/")
	v, err := h.Store.Schedule(id)
	if err != nil {
		fail(w, errors.New("schedule not found"), 404)
		return
	}
	if r.Method == "DELETE" {
		if err = h.Store.DeleteSchedule(id); err != nil {
			fail(w, err, 500)
			return
		}
		reply(w, map[string]bool{"ok": true})
		return
	}
	if r.Method != "PATCH" {
		fail(w, errors.New("endpoint not found"), 404)
		return
	}
	var b struct {
		Enabled     bool   `json:"enabled"`
		Confirm     bool   `json:"confirm"`
		PreviewTask string `json:"previewTask"`
	}
	if err = decode(w, r, &b); err != nil {
		fail(w, err, 400)
		return
	}
	if b.Enabled {
		node, e := h.Store.Node(v.Node)
		if e != nil || time.Since(node.LastSeen) > 45*time.Second {
			fail(w, errors.New("node offline"), 409)
			return
		}
		if !b.Confirm || v.Phase != "" || h.Store.Busy(v.Node) {
			fail(w, errors.New("review and explicit scheduled deletion confirmation required"), 409)
			return
		}
		t, e := h.Store.Task(b.PreviewTask)
		if e != nil {
			fail(w, errors.New("fresh matching preview required"), 409)
			return
		}
		if _, e = previewPlan(t, v.Node, v.Rule, time.Now()); e != nil {
			fail(w, e, 409)
			return
		}
		v.Next = time.Now().Add(time.Duration(v.Hours) * time.Hour)
		v.Outcome = "enabled"
	} else {
		v.Outcome = "paused"
		// Already-submitted destructive tasks continue; a queued preview cannot
		// authorize later execution after disabling.
		if v.Phase == "preview" {
			v.Phase = ""
			v.Task = ""
		}
	}
	v.Enabled = b.Enabled
	if err = h.Store.SaveSchedule(v); err != nil {
		fail(w, err, 500)
		return
	}
	reply(w, v)
}

func (h *Hub) RunSchedules(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := h.scheduleTick(now); err != nil {
				log.Print("scheduled cleanup processing failed")
			}
		}
	}
}

// Serialized with API admission and Agent polling. At most two pipelines run,
// each using the existing node-owned preview and one-use execution protocol.
func (h *Hub) scheduleTick(now time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping || h.workContext().Err() != nil {
		return nil
	}
	schedules, err := h.Store.Schedules()
	if err != nil {
		return err
	}
	active := 0
	for _, v := range schedules {
		if v.Phase != "" {
			active++
		}
	}
	for _, v := range schedules {
		if v.Phase != "" {
			t, e := h.Store.Task(v.Task)
			if e == nil && (t.Status == "pending" || t.Status == "running") {
				continue
			}
			if e != nil || t.Status != "succeeded" {
				v.Enabled = false
				v.Phase = ""
				v.Outcome = "failed"
				if err = h.Store.SaveSchedule(v); err != nil {
					return err
				}
				active--
				continue
			}
			if v.Phase == "execute" {
				v.Phase = ""
				v.Outcome = "succeeded"
				if err = h.Store.SaveSchedule(v); err != nil {
					return err
				}
				active--
				continue
			}
			plan, e := previewPlan(t, v.Node, v.Rule, now)
			node, nodeErr := h.Store.Node(v.Node)
			if !v.Enabled || e != nil || nodeErr != nil || now.Sub(node.LastSeen) > 45*time.Second || h.Store.Busy(v.Node) {
				v.Enabled = false
				v.Phase = ""
				v.Outcome = "review_required"
				if err = h.Store.SaveSchedule(v); err != nil {
					return err
				}
				active--
				continue
			}
			if len(plan.Files) == 0 {
				v.Phase = ""
				v.Outcome = "empty"
				if err = h.Store.SaveSchedule(v); err != nil {
					return err
				}
				active--
				continue
			}
			v.Phase = "execute"
			if err = h.scheduleDispatch(v, engine.Request{Kind: "execute", PlanID: plan.ID}, now); err != nil {
				return err
			}
			continue
		}
		if !v.Enabled || now.Before(v.Next) {
			continue
		}
		// Consume the due tick, even if offline or busy. No catch-up burst or retry.
		v.Next = now.Add(time.Duration(v.Hours) * time.Hour)
		node, nodeErr := h.Store.Node(v.Node)
		if nodeErr != nil || now.Sub(node.LastSeen) > 45*time.Second {
			v.Outcome = "offline"
		} else if h.Store.Busy(v.Node) || active >= 2 {
			v.Outcome = "busy"
		} else {
			v.Phase = "preview"
			if err = h.scheduleDispatch(v, engine.Request{Kind: "preview", Rule: v.Rule}, now); err != nil {
				return err
			}
			active++
			continue
		}
		if err = h.Store.SaveSchedule(v); err != nil {
			return err
		}
	}
	return nil
}
func (h *Hub) scheduleDispatch(v store.Schedule, r engine.Request, now time.Time) error {
	audit, err := h.Store.BeginAudit(store.AuditEvent{Actor: "scheduler", Role: "system", Action: "schedules." + r.Kind, Target: v.ID})
	if err != nil {
		return err
	} // Persist authorization evidence before any task.
	t := engine.Task{ID: engine.ID(), Node: v.Node, Status: "pending", Created: now, Request: r}
	v.Task = t.ID
	v.Outcome = "running"
	if err = h.Store.ScheduleTask(v, t); err != nil {
		return err
	}
	if err = h.Store.FinishAudit(audit, 200); err != nil {
		log.Print("scheduled audit outcome unknown")
	}
	if v.Node == "local" && h.Engine != nil {
		h.workers.Add(1)
		go h.localTask(t)
	}
	return nil
}
