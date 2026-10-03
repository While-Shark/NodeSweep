package hub

import (
	"encoding/json"
	"errors"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const adminBodyLimit = 64 << 10
const pollBodyLimit = 32 << 20

func bearer(header string) string {
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	token := strings.TrimPrefix(header, "Bearer ")
	if token == "" || len(token) > 256 || strings.ContainsAny(token, " \t\r\n") {
		return ""
	}
	return token
}

func validatePoll(b Poll) error {
	if b.TaskID != "" && (!b.Busy || !engine.ValidID(b.TaskID)) {
		return errors.New("invalid active task metadata")
	}
	if p := b.Progress; p != nil {
		if b.TaskID == "" || !b.Busy || p.Visited < 0 || p.Visited > 100000 || p.Files < 0 || p.Files > p.Visited || p.Bytes < 0 || p.Limit < 1 || p.Limit > 100000 || p.ElapsedMillis < 0 || p.ElapsedMillis > 180000 {
			return errors.New("invalid scan progress")
		}
	}

	if len(b.Roots) > 64 || len(b.ScanRoots) > 64 || len(b.Metrics.Disks) > 128 || len(b.Metrics.Host) > 255 || len(b.Metrics.Load) > 100 || len(b.Metrics.Uptime) > 100 {
		return errors.New("node metadata exceeds limits")
	}
	paths := append(append([]string{}, b.Roots...), b.ScanRoots...)
	for _, d := range b.Metrics.Disks {
		paths = append(paths, d.Path)
		if d.Available > d.Total || d.FreeInodes > d.Inodes {
			return errors.New("invalid node metrics")
		}
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) || len(p) > 4096 || strings.ContainsRune(p, 0) {
			return errors.New("invalid node metadata path")
		}
	}
	if math.IsNaN(b.Metrics.CPU) || math.IsInf(b.Metrics.CPU, 0) || b.Metrics.CPU < 0 || b.Metrics.CPU > 100 || b.Metrics.MemoryAvailable > b.Metrics.MemoryTotal {
		return errors.New("invalid node metrics")
	}
	metadata := b
	metadata.Result = nil
	raw, err := json.Marshal(metadata)
	if err != nil || len(raw) > adminBodyLimit {
		return errors.New("node metadata exceeds limits")
	}
	if b.Result != nil && (len(b.Result.ID) > 64 || len(b.Result.Error) > 8192) {
		return errors.New("task completion exceeds limits")
	}
	return nil
}

type pollGate struct {
	mu      sync.Mutex
	entries map[string]*pollAllowance
}
type pollAllowance struct {
	tokens float64
	at     time.Time
	active bool
}

// One body in flight per authenticated node; short bursts support result retries.
// Unknown credentials never allocate limiter state. Idle entries expire.
func (g *pollGate) enter(node string) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if g.entries == nil {
		g.entries = map[string]*pollAllowance{}
	}
	for id, state := range g.entries {
		if !state.active && now.Sub(state.at) > 10*time.Minute {
			delete(g.entries, id)
		}
	}
	state := g.entries[node]
	if state == nil {
		state = &pollAllowance{tokens: 6, at: now}
		g.entries[node] = state
	}
	state.tokens = math.Min(6, state.tokens+now.Sub(state.at).Seconds()*0.6)
	state.at = now
	if state.active || state.tokens < 1 {
		return nil, false
	}
	state.tokens--
	state.active = true
	return func() { g.mu.Lock(); state.active = false; g.mu.Unlock() }, true
}
