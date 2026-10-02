package hub

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
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

	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
)

type Hub struct {
	Store    *store.Store
	Engine   *engine.Engine
	Token    string
	mu       sync.Mutex
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
	case p == "nodes" && r.Method == "GET":
		v, e := h.Store.Nodes()
		if e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, v)
	case p == "nodes" && r.Method == "POST":
		var b struct {
			Name string `json:"name"`
		}
		if e := decode(w, r, &b); e != nil {
			fail(w, e, 400)
			return
		}
		if len(strings.TrimSpace(b.Name)) < 1 || len(b.Name) > 100 {
			fail(w, errors.New("name required, max 100 characters"), 400)
			return
		}
		token := engine.ID() + engine.ID()
		n := store.Node{ID: engine.ID(), Name: b.Name}
		if e := h.Store.AddNode(n, Hash(token)); e != nil {
			fail(w, e, 500)
			return
		}
		reply(w, map[string]any{"node": n, "token": token})
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
		n := store.Node{ID: "local", Name: "本机", LastSeen: time.Now(), Metrics: sampler.Read(), Roots: h.Engine.Roots, ScanRoots: h.Engine.ScanRoots}
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
	Node      string         `json:"node"`
	Metrics   engine.Metrics `json:"metrics"`
	Roots     []string       `json:"roots"`
	ScanRoots []string       `json:"scanRoots"`
	Result    *engine.Task   `json:"result,omitempty"`
	Busy      bool           `json:"busy"`
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
	n.LastSeen = time.Now()
	n.Metrics = b.Metrics
	n.Roots = b.Roots
	n.ScanRoots = b.ScanRoots
	if e = h.Store.UpdateNode(n); e != nil {
		fail(w, e, 500)
		return
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
	reply(w, map[string]any{"task": task})
}

func (h *Hub) workContext() context.Context {
	if h.Context != nil {
		return h.Context
	}
	return context.Background()
}

// Stop admission before waiting so Add and Wait cannot race during shutdown.
func (h *Hub) Wait() { h.mu.Lock(); h.stopping = true; h.mu.Unlock(); h.workers.Wait() }
