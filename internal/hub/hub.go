package hub

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/While-Shark/NodeSweep/internal/alerts"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
)

type Hub struct {
	Alerts   *alerts.Service
	Store    *store.Store
	Engine   *engine.Engine
	Token    string
	mu       sync.Mutex
	cancels  map[string]context.CancelFunc
	Context  context.Context
	workers  sync.WaitGroup
	stopping bool
	polls    pollGate
}

func Hash(s string) string   { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func equal(a, b string) bool { return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Print(err)
	}
}
func fail(w http.ResponseWriter, err error, code int) {
	if code >= 500 {
		log.Printf("request failed: %v", err)
		err = errors.New("internal server error")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	return decodeLimit(w, r, v, adminBodyLimit)
}
func decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
func (h *Hub) Handler(assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /agent/poll", h.poll)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		credential := bearer(r.Header.Get("Authorization"))
		if len(h.Token) < 32 || credential == "" || !equal(Hash(credential), Hash(h.Token)) {
			fail(w, errors.New("invalid administrator token"), 401)
			return
		}
		h.api(w, r)
	})
	mux.Handle("/", http.FileServerFS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
func (h *Hub) api(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case p == "alerts" && r.Method == "GET":
		if h.Alerts == nil {
			fail(w, errors.New("alerts unavailable"), 503)
			return
		}
		settings, e := h.Alerts.Settings()
		if e != nil {
			fail(w, e, 500)
			return
		}
		events, e := h.Alerts.Events()
		if e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, map[string]any{"settings": settings, "events": events, "webhookConfigured": h.Alerts.Webhook != ""})
	case p == "alerts" && r.Method == "PUT":
		if h.Alerts == nil {
			fail(w, errors.New("alerts unavailable"), 503)
			return
		}
		var settings alerts.Settings
		if e := decode(w, r, &settings); e != nil {
			fail(w, e, 400)
			return
		}
		if e := h.Alerts.Save(settings); e != nil {
			fail(w, e, 400)
			return
		}
		reply(w, map[string]bool{"ok": true})
	case strings.HasPrefix(p, "metrics/") && r.Method == "GET":
		node := strings.TrimPrefix(p, "metrics/")
		if _, err := h.Store.Node(node); errors.Is(err, sql.ErrNoRows) {
			fail(w, errors.New("node not found"), 404)
			return
		} else if err != nil {
			fail(w, err, 500)
			return
		}
		period := r.URL.Query().Get("period")
		if period != "" && period != "24h" && period != "7d" {
			fail(w, errors.New("invalid history period"), 400)
			return
		}
		points, err := h.Store.MetricsHistory(node, time.Now(), period == "7d")
		if err != nil {
			fail(w, err, 500)
			return
		}
		reply(w, points)
	case p == "nodes" && r.Method == "GET":
		v, e := h.Store.Nodes()
		if e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, v)
	case p == "nodes" && r.Method == "POST":
		var b struct {
			Name  string `json:"name"`
			Group string `json:"group"`
		}
		if e := decode(w, r, &b); e != nil {
			fail(w, e, 400)
			return
		}
		if !validMetadata(b.Name, b.Group) {
			fail(w, errors.New("invalid node name or group"), 400)
			return
		}
		token := engine.ID() + engine.ID()
		n := store.Node{ID: engine.ID(), Name: strings.TrimSpace(b.Name), Group: strings.TrimSpace(b.Group)}
		if e := h.Store.AddNode(n, Hash(token)); e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, map[string]any{"node": n, "token": token})
	case strings.HasPrefix(p, "nodes/") && r.Method == "PATCH":
		var b struct {
			Name  string `json:"name"`
			Group string `json:"group"`
		}
		if e := decode(w, r, &b); e != nil {
			fail(w, e, 400)
			return
		}
		if !validMetadata(b.Name, b.Group) {
			fail(w, errors.New("invalid node name or group"), 400)
			return
		}
		e := h.Store.SetNodeMetadata(strings.TrimPrefix(p, "nodes/"), strings.TrimSpace(b.Name), strings.TrimSpace(b.Group))
		if errors.Is(e, sql.ErrNoRows) {
			fail(w, errors.New("node not found"), 404)
			return
		}
		if e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, map[string]bool{"ok": true})
	case strings.HasPrefix(p, "nodes/") && r.Method == "DELETE":
		h.mu.Lock()
		defer h.mu.Unlock()
		if e := h.Store.DeleteNode(strings.TrimPrefix(p, "nodes/")); e != nil {
			fail(w, e, 400)
			return
		}
		reply(w, map[string]bool{"ok": true})
	case p == "rules/export" && r.Method == "GET":
		rules, e := h.Store.Rules()
		if e != nil {
			fail(w, e, 500)
			return
		}
		if scheme := r.URL.Query().Get("scheme"); scheme != "" {
			filtered := []engine.Rule{}
			for _, rule := range rules {
				name := rule.Scheme
				if name == "" {
					name = "默认方案"
				}
				if name == scheme {
					filtered = append(filtered, rule)
				}
			}
			rules = filtered
		}
		bundle := engine.RuleBundle{Format: engine.BundleFormat, Version: engine.BundleVersion, Rules: rules}
		if e = bundle.Validate(); e != nil {
			fail(w, errors.New("export requires 1–100 rules; select a smaller scheme"), 400)
			return
		}
		for i := range bundle.Rules {
			bundle.Rules[i].ID = ""
		}
		encoded, e := json.MarshalIndent(bundle, "", "  ")
		if e != nil {
			fail(w, e, 500)
			return
		}
		if len(encoded)+1 > 256<<10 {
			fail(w, errors.New("export exceeds file size limit; select a smaller scheme"), 400)
			return
		}
		reply(w, bundle)

	case p == "rules/import" && r.Method == "POST":
		// Transfer files are small configuration objects, not task result trees.
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
		var bundle engine.RuleBundle
		if e := decodeLimit(w, r, &bundle, 256<<10); e != nil {
			fail(w, e, 400)
			return
		}
		result, e := h.Store.ImportRules(bundle)
		if e != nil {
			fail(w, e, 400)
			return
		}
		reply(w, result)
	case p == "rules" && r.Method == "GET":
		v, e := h.Store.Rules()
		if e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, v)
	case p == "rules" && r.Method == "POST":
		var b engine.Rule
		if e := decode(w, r, &b); e != nil {
			fail(w, e, 400)
			return
		}
		if e := engine.ValidateRule(b); e != nil {
			fail(w, e, 400)
			return
		}
		if b.ID == "" {
			b.ID = engine.ID()
		}
		if e := h.Store.SaveRule(b); e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, b)
	case strings.HasPrefix(p, "rules/") && r.Method == "DELETE":
		if e := h.Store.DeleteRule(strings.TrimPrefix(p, "rules/")); e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, map[string]bool{"ok": true})
	case p == "tasks" && r.Method == "GET":
		v, e := h.Store.Tasks()
		if e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, v)
	case strings.HasPrefix(p, "tasks/") && strings.HasSuffix(p, "/cancel") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(p, "tasks/"), "/cancel")
		h.mu.Lock()
		defer h.mu.Unlock()
		original, e := h.Store.Task(id)
		if e != nil {
			fail(w, errors.New("task not found"), 404)
			return
		}
		node, e := h.Store.Node(original.Node)
		if e != nil || (original.Node != "local" && !node.ScanControl) || (original.Node == "local" && h.Engine == nil) {
			fail(w, errors.New("agent upgrade required for scan cancellation"), 409)
			return
		}
		result, e := h.Store.CancelScan(id)
		if e != nil {
			fail(w, e, 409)
			return
		}
		if cancel := h.cancels[id]; cancel != nil {
			cancel()
		}
		reply(w, result)
	case strings.HasPrefix(p, "tasks/") && r.Method == "GET":
		v, e := h.Store.Task(strings.TrimPrefix(p, "tasks/"))
		if e != nil {
			fail(w, errors.New("task not found"), 404)
			return
		}
		reply(w, v)
	case p == "tasks" && r.Method == "POST":
		var b struct {
			Node    string         `json:"node"`
			Request engine.Request `json:"request"`
		}
		if e := decode(w, r, &b); e != nil {
			fail(w, e, 400)
			return
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.stopping || h.workContext().Err() != nil {
			fail(w, errors.New("server shutting down"), 503)
			return
		}
		n, e := h.Store.Node(b.Node)
		if e != nil {
			fail(w, errors.New("node not found"), 404)
			return
		}
		if time.Since(n.LastSeen) > 45*time.Second {
			fail(w, errors.New("node offline"), 409)
			return
		}
		if e := engine.ValidateRequest(b.Request); e != nil {
			fail(w, e, 400)
			return
		}
		if h.Store.Busy(b.Node) {
			fail(w, errors.New("node already has an active task"), 409)
			return
		}
		t := engine.Task{ID: engine.ID(), Node: b.Node, Request: b.Request, Status: "pending", Created: time.Now()}
		if e = h.Store.CreateTask(t); e != nil {
			fail(w, e, 500)
			return
		}
		if b.Node == "local" && h.Engine != nil {
			h.workers.Add(1)
			go h.localTask(t)
		}
		reply(w, t)
	default:
		fail(w, errors.New("endpoint not found"), 404)
	}
}
func (h *Hub) localTask(t engine.Task) {
	defer h.workers.Done()
	claimed, err := h.Store.Claim(t.ID, t.Node)
	if err != nil {
		log.Print(err)
		return
	}
	if claimed == nil {
		return
	}
	ctx, cancel := context.WithTimeout(h.workContext(), 90*time.Second)
	defer cancel()
	h.mu.Lock()
	if h.cancels == nil {
		h.cancels = map[string]context.CancelFunc{}
	}
	h.cancels[t.ID] = cancel
	fresh, checkErr := h.Store.Task(t.ID)
	if checkErr != nil || fresh.CancelRequested {
		cancel()
	}
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.cancels, t.ID); h.mu.Unlock() }()
	if t.Request.Kind == "scan" {
		ctx = engine.WithScanProgress(ctx, func(progress engine.ScanProgress) {
			if e := h.Store.ScanProgress(t.ID, t.Node, progress); e != nil {
				log.Print(e)
			}
		})
	}
	t.Result, err = h.Engine.Run(ctx, t.Request)
	t.Status = "succeeded"
	if err != nil {
		t.Status = "failed"
		t.Error = err.Error()
	}
	if err = h.Store.Complete(t); err != nil {
		log.Print(err)
	}
}
func (h *Hub) LocalMetrics(ctx context.Context) {
	if h.Engine == nil {
		return
	}
	if _, err := h.Store.Node("local"); err != nil {
		if err = h.Store.AddNode(store.Node{ID: "local", Name: "本机"}, ""); err != nil {
			log.Print(err)
			return
		}
	}
	sampler := engine.Sampler{}
	for {
		n := store.Node{ID: "local", Name: "本机", ScanControl: true, LastSeen: time.Now(), Metrics: sampler.Read(), Roots: h.Engine.Roots, ScanRoots: h.Engine.ScanRoots}
		if err := h.Store.UpdateNode(n); err != nil {
			log.Print(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

type Poll struct {
	ScanControl bool                 `json:"scanControl,omitempty"`
	TaskID      string               `json:"taskId,omitempty"`
	Progress    *engine.ScanProgress `json:"progress,omitempty"`
	Node        string               `json:"node"`
	Metrics     engine.Metrics       `json:"metrics"`
	Roots       []string             `json:"roots"`
	ScanRoots   []string             `json:"scanRoots"`
	Result      *engine.Task         `json:"result,omitempty"`
	Busy        bool                 `json:"busy"`
}

func (h *Hub) poll(w http.ResponseWriter, r *http.Request) {
	nodeID := r.Header.Get("X-Node-ID")
	credential := bearer(r.Header.Get("Authorization"))
	if nodeID == "" || len(nodeID) > 64 || nodeID == "local" || credential == "" || !equal(h.Store.Token(nodeID), Hash(credential)) {
		fail(w, errors.New("invalid node credentials"), 401)
		return
	}

	release, ok := h.polls.enter(nodeID)
	if !ok {
		w.Header().Set("Retry-After", "5")
		fail(w, errors.New("node poll rate exceeded"), 429)
		return
	}
	defer release()
	var b Poll
	if e := decodeLimit(w, r, &b, pollBodyLimit); e != nil {
		fail(w, e, 400)
		return
	}
	if b.Node != nodeID || b.Node == "local" || !equal(h.Store.Token(b.Node), Hash(credential)) {
		fail(w, errors.New("invalid node credentials"), 401)
		return
	}
	if e := validatePoll(b); e != nil {
		fail(w, e, 400)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, e := h.Store.Node(b.Node)
	if e != nil {
		fail(w, e, 404)
		return
	}
	n.ScanControl = b.ScanControl
	n.LastSeen = time.Now()
	n.Metrics = b.Metrics
	n.Roots = b.Roots
	n.ScanRoots = b.ScanRoots
	if e = h.Store.UpdateNode(n); e != nil {
		fail(w, e, 500)
		return
	}
	if b.Progress != nil {
		if e := h.Store.ScanProgress(b.TaskID, b.Node, *b.Progress); e != nil {
			fail(w, e, 400)
			return
		}
	}
	if b.Result != nil {
		completion := *b.Result
		completion.Node = b.Node // Never trust a node identity embedded in a result.
		if e = h.Store.Complete(completion); e != nil {
			fail(w, e, 400)
			return
		}
	}

	var task *engine.Task
	if !b.Busy && h.workContext().Err() == nil {
		task, e = h.Store.Next(b.Node)
		if e != nil {
			fail(w, e, 500)
			return
		}
	}
	response := map[string]any{"task": task}
	if b.ScanControl {
		if id := h.Store.ScanCancellation(b.Node); id != "" {
			response["cancel"] = id
		}
	}
	reply(w, response)
}

func (h *Hub) workContext() context.Context {
	if h.Context != nil {
		return h.Context
	}
	return context.Background()
}

// Stop admission before waiting so Add and Wait cannot race during shutdown.
func (h *Hub) Wait() { h.mu.Lock(); h.stopping = true; h.mu.Unlock(); h.workers.Wait() }

func validMetadata(name, group string) bool {
	if !utf8.ValidString(name) || !utf8.ValidString(group) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(group) > 64 {
		return false
	}
	for _, r := range name + group {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
