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
	Store  *store.Store
	Engine *engine.Engine
	Token  string
	mu     sync.Mutex
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
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
		if !equal(Hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), Hash(h.Token)) {
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
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
		if e := h.Store.DeleteNode(strings.TrimPrefix(p, "nodes/")); e != nil {
			fail(w, e, 400)
			return
		}
		reply(w, map[string]bool{"ok": true})
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
		n, e := h.Store.Node(b.Node)
		if e != nil {
			fail(w, errors.New("node not found"), 404)
			return
		}
		if time.Since(n.LastSeen) > 45*time.Second {
			fail(w, errors.New("node offline"), 409)
			return
		}
		switch b.Request.Kind {
		case "scan", "preview", "execute", "detect":
		default:
			fail(w, errors.New("unsupported operation"), 400)
			return
		}
		if h.Store.Busy(b.Node) {
			fail(w, errors.New("node already has an active task"), 409)
			return
		}
		t := engine.Task{ID: engine.ID(), Node: b.Node, Request: b.Request, Status: "pending", Created: time.Now()}
		if e = h.Store.SaveTask(t); e != nil {
			fail(w, e, 500)
			return
		}
		if b.Node == "local" && h.Engine != nil {
			go h.localTask(t)
		}
		reply(w, t)
	default:
		fail(w, errors.New("endpoint not found"), 404)
	}
}
func (h *Hub) localTask(t engine.Task) {
	t.Status = "running"
	if err := h.Store.SaveTask(t); err != nil {
		log.Print(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := h.Engine.Run(ctx, t.Request)
	t.Result = result
	t.Status = "succeeded"
	if err != nil {
		t.Status = "failed"
		t.Error = err.Error()
	}
	if err = h.Store.SaveTask(t); err != nil {
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
	if nodeID == "" || nodeID == "local" || !equal(h.Store.Token(nodeID), Hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))) {
		fail(w, errors.New("invalid node credentials"), 401)
		return
	}

	var b Poll
	if e := decode(w, r, &b); e != nil {
		fail(w, e, 400)
		return
	}
	if b.Node != nodeID || b.Node == "local" || !equal(h.Store.Token(b.Node), Hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))) {
		fail(w, errors.New("invalid node credentials"), 401)
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
		original, e := h.Store.Task(b.Result.ID)
		if e != nil || original.Node != b.Node {
			fail(w, errors.New("task ownership mismatch"), 400)
			return
		}
		if original.Status == "running" || original.Status == "interrupted" {
			original.Result = b.Result.Result
			original.Error = b.Result.Error
			original.Status = b.Result.Status
			if original.Status != "succeeded" && original.Status != "failed" {
				fail(w, errors.New("invalid completion status"), 400)
				return
			}
			if e = h.Store.SaveTask(original); e != nil {
				fail(w, e, 500)
				return
			}
		}
	}
	var task *engine.Task
	if !b.Busy {
		task, e = h.Store.Next(b.Node)
		if e != nil {
			fail(w, e, 500)
			return
		}
	}
	reply(w, map[string]any{"task": task})
}
