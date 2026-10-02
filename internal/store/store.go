package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/While-Shark/NodeSweep/internal/engine"
	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }
type Node struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	LastSeen  time.Time      `json:"lastSeen"`
	Metrics   engine.Metrics `json:"metrics"`
	Roots     []string       `json:"roots"`
	ScanRoots []string       `json:"scanRoots"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS nodes(id TEXT PRIMARY KEY,name TEXT NOT NULL,token TEXT NOT NULL,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS rules(id TEXT PRIMARY KEY,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS tasks(id TEXT PRIMARY KEY,node TEXT NOT NULL,status TEXT NOT NULL,created TEXT NOT NULL,body TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS tasks_queue ON tasks(node,status,created);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db}
	// Never replay destructive work after a hub restart. Preserve uncertainty.
	rows, err := db.Query("SELECT body FROM tasks WHERE status IN ('running','pending')")
	if err != nil {
		return nil, err
	}
	var interrupted []engine.Task
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		var t engine.Task
		if err = json.Unmarshal([]byte(raw), &t); err != nil {
			rows.Close()
			return nil, err
		}
		interrupted = append(interrupted, t)
	}
	rows.Close()
	for _, t := range interrupted {
		t.Status = "interrupted"
		t.Error = "hub restarted; inspect node and create a new preview before retrying"
		if err = s.SaveTask(t); err != nil {
			return nil, err
		}
	}
	return s, nil
}
func (s *Store) Nodes() ([]Node, error) {
	rows, e := s.DB.Query("SELECT body FROM nodes ORDER BY name")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		var n Node
		if e = json.Unmarshal([]byte(raw), &n); e != nil {
			return nil, e
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
func (s *Store) Node(id string) (Node, error) {
	var n Node
	var raw string
	e := s.DB.QueryRow("SELECT body FROM nodes WHERE id=?", id).Scan(&raw)
	if e != nil {
		return n, e
	}
	e = json.Unmarshal([]byte(raw), &n)
	return n, e
}
func (s *Store) AddNode(n Node, hash string) error {
	b, e := json.Marshal(n)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec("INSERT INTO nodes(id,name,token,body) VALUES(?,?,?,?)", n.ID, n.Name, hash, string(b))
	return e
}
func (s *Store) UpdateNode(n Node) error {
	b, e := json.Marshal(n)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec("UPDATE nodes SET body=? WHERE id=?", string(b), n.ID)
	return e
}
func (s *Store) Token(id string) string {
	var t string
	_ = s.DB.QueryRow("SELECT token FROM nodes WHERE id=?", id).Scan(&t)
	return t
}
func (s *Store) DeleteNode(id string) error {
	if id == "local" {
		return errors.New("local node cannot be revoked")
	}
	_, e := s.DB.Exec("DELETE FROM nodes WHERE id=?", id)
	return e
}
func (s *Store) Rules() ([]engine.Rule, error) {
	rows, e := s.DB.Query("SELECT body FROM rules ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []engine.Rule{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		var r engine.Rule
		if e = json.Unmarshal([]byte(raw), &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) SaveRule(r engine.Rule) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec("INSERT INTO rules VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", r.ID, string(b))
	return e
}
func (s *Store) DeleteRule(id string) error {
	_, e := s.DB.Exec("DELETE FROM rules WHERE id=?", id)
	return e
}
func (s *Store) SaveTask(t engine.Task) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec("INSERT INTO tasks VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,body=excluded.body", t.ID, t.Node, t.Status, t.Created.UTC().Format(time.RFC3339Nano), string(b))
	return e
}
func (s *Store) Task(id string) (engine.Task, error) {
	var t engine.Task
	var b string
	e := s.DB.QueryRow("SELECT body FROM tasks WHERE id=?", id).Scan(&b)
	if e != nil {
		return t, e
	}
	e = json.Unmarshal([]byte(b), &t)
	return t, e
}
func (s *Store) Tasks() ([]engine.Task, error) {
	rows, e := s.DB.Query("SELECT body FROM tasks ORDER BY created DESC LIMIT 100")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []engine.Task{}
	for rows.Next() {
		var b string
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var t engine.Task
		if e = json.Unmarshal([]byte(b), &t); e != nil {
			return nil, e
		}
		t.Result = nil
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) Next(node string) (*engine.Task, error) {
	var b string
	e := s.DB.QueryRow("SELECT body FROM tasks WHERE node=? AND status='pending' ORDER BY created LIMIT 1", node).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var t engine.Task
	if e = json.Unmarshal([]byte(b), &t); e != nil {
		return nil, e
	}
	t.Status = "running"
	if e = s.SaveTask(t); e != nil {
		return nil, e
	}
	return &t, nil
}
func (s *Store) Busy(node string) bool {
	var count int
	err := s.DB.QueryRow("SELECT count(*) FROM tasks WHERE node=? AND status IN ('pending','running')", node).Scan(&count)
	return err != nil || count > 0
}

// Large scan results expire; operation summaries stay visible for 30 days.
func (s *Store) Prune() error {
	_, e := s.DB.Exec("DELETE FROM tasks WHERE created < ? AND status NOT IN ('pending','running')", time.Now().Add(-30*24*time.Hour).UTC().Format(time.RFC3339Nano))
	return e
}

// Expire large scan payloads while retaining a small auditable task summary.
func (s *Store) CompactScans() error {
	rows, err := s.DB.Query("SELECT body FROM tasks WHERE status='succeeded' AND json_extract(body,'$.request.kind')='scan' AND json_type(body,'$.result') NOT IN ('null') ORDER BY created DESC")
	if err != nil {
		return err
	}
	var compact []engine.Task
	count := 0
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var t engine.Task
		if err = json.Unmarshal([]byte(raw), &t); err != nil {
			rows.Close()
			return err
		}
		if t.Request.Kind == "scan" && t.Result != nil {
			count++
			if count > 10 {
				t.Result = nil
				t.Error = "scan payload expired; run a new scan"
				compact = append(compact, t)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, t := range compact {
		if err = s.SaveTask(t); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ExpireTasks() error {
	rows, err := s.DB.Query("SELECT body FROM tasks WHERE status IN ('pending','running') AND created < ?", time.Now().Add(-3*time.Minute).UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	var tasks []engine.Task
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var t engine.Task
		if err = json.Unmarshal([]byte(raw), &t); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		t.Status = "interrupted"
		t.Error = "task timed out; execution state may be unknown, inspect node before retrying"
		if err = s.SaveTask(t); err != nil {
			return err
		}
	}
	return nil
}
