package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/While-Shark/NodeSweep/internal/engine"
	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }
type Node struct {
	LogChecks   bool           `json:"logChecks"`
	ScanControl bool           `json:"scanControl"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Group       string         `json:"group"`
	LastSeen    time.Time      `json:"lastSeen"`
	Metrics     engine.Metrics `json:"metrics"`
	Roots       []string       `json:"roots"`
	ScanRoots   []string       `json:"scanRoots"`
}

func Open(path string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	directory, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	owner, ok := directory.Sys().(*syscall.Stat_t)
	if !ok || !directory.IsDir() || directory.Mode().Perm()&0022 != 0 || int(owner.Uid) != os.Geteuid() {
		return nil, errors.New("database directory must be owned by this process user and not writable by other users")
	}
	if err := secureStateFile(path, true); err != nil {
		return nil, err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if err := secureStateFile(path+suffix, false); err != nil {
			return nil, err
		}
	}
	// Escape URI metacharacters so a filename cannot add SQLite DSN options.
	dsn := (&url.URL{Scheme: "file", Path: path}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS audit_events(id INTEGER PRIMARY KEY AUTOINCREMENT,at TEXT NOT NULL,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS nodes(id TEXT PRIMARY KEY,name TEXT NOT NULL,token TEXT NOT NULL,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS schedules(id TEXT PRIMARY KEY,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS rules(id TEXT PRIMARY KEY,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS tasks(id TEXT PRIMARY KEY,node TEXT NOT NULL,status TEXT NOT NULL,created TEXT NOT NULL,body TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS tasks_queue ON tasks(node,status,created);
 CREATE TABLE IF NOT EXISTS metric_samples(node TEXT NOT NULL,minute INTEGER NOT NULL,body TEXT NOT NULL,PRIMARY KEY(node,minute));
 CREATE INDEX IF NOT EXISTS metric_samples_time ON metric_samples(minute);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db}
	// Never replay destructive work after a hub restart. A single conditional
	// update avoids restoring a stale task snapshot over a newer result.
	_, err = db.Exec(`UPDATE tasks SET status='interrupted',
        body=json_set(body,'$.status','interrupted','$.error',?)
        WHERE status IN ('running','pending')`,
		"hub restarted; inspect node and create a new preview before retrying")
	if err != nil {
		db.Close()
		return nil, err
	}

	// Unattended deletion never resumes automatically after a restart. A new
	// review is required; task records retain any uncertain in-flight outcome.
	if _, err = db.Exec(`UPDATE schedules SET body=json_set(body,'$.enabled',json('false'),'$.phase','','$.outcome','restart_review_required')`); err != nil {
		db.Close()
		return nil, err
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
	_, e = s.DB.Exec(`UPDATE nodes SET body=json_set(body,
 "$.lastSeen",json_extract(?,"$.lastSeen"),
 "$.metrics",json_extract(?,"$.metrics"),
 "$.roots",json_extract(?,"$.roots"),
 "$.scanRoots",json_extract(?,"$.scanRoots"),
 "$.logChecks",json(CASE json_extract(?,"$.logChecks") WHEN 1 THEN 'true' ELSE 'false' END),
 "$.scanControl",json(CASE json_extract(?,"$.scanControl") WHEN 1 THEN 'true' ELSE 'false' END)) WHERE id=?`, string(b), string(b), string(b), string(b), string(b), string(b), n.ID)
	if e != nil {
		return e
	}
	return s.recordMetrics(n.ID, n.Metrics, time.Now())
}

// Metadata is updated separately so stale metric samples cannot overwrite admin edits.
func (s *Store) SetNodeMetadata(id, name, group string) error {
	result, err := s.DB.Exec(`UPDATE nodes SET name=?,body=json_set(body,'$.name',?,'$.group',?) WHERE id=?`, name, name, group, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
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
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM metric_samples WHERE node=?", id); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE schedules SET body=json_set(body,'$.enabled',json('false'),'$.phase','','$.outcome','revoked') WHERE json_extract(body,'$.node')=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM nodes WHERE id=?", id); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE tasks SET status='interrupted', body=json_set(body,'$.status','interrupted','$.error',?)
 WHERE node=? AND status IN ('pending','running')`, "node revoked; in-flight execution may have completed, inspect the server", id); err != nil {
		return err
	}
	return tx.Commit()
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
