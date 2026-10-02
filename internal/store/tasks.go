package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/While-Shark/NodeSweep/internal/engine"
)

// CreateTask never overwrites an existing task or its immutable request.
func (s *Store) CreateTask(t engine.Task) error {
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO tasks VALUES(?,?,?,?,?)", t.ID, t.Node, t.Status, t.Created.UTC().Format(time.RFC3339Nano), string(body))
	return err
}
func (s *Store) Task(id string) (engine.Task, error) {
	var t engine.Task
	var body string
	if err := s.DB.QueryRow("SELECT body FROM tasks WHERE id=?", id).Scan(&body); err != nil {
		return t, err
	}
	err := json.Unmarshal([]byte(body), &t)
	return t, err
}
func (s *Store) Tasks() ([]engine.Task, error) {
	// Do not load large treemaps merely to remove them in Go afterwards.
	rows, err := s.DB.Query("SELECT json_remove(body,'$.result') FROM tasks ORDER BY created DESC,id DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []engine.Task{}
	for rows.Next() {
		var body string
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		var t engine.Task
		if err = json.Unmarshal([]byte(body), &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Claim and Next perform compare-and-set in one SQL statement. Expiry or another
// poll cannot slip between selecting a task and marking it running.
func (s *Store) Claim(id, node string) (*engine.Task, error) {
	return readClaim(s.DB.QueryRow(`UPDATE tasks SET status='running', body=json_set(body,'$.status','running')
 WHERE id=? AND node=? AND status='pending'
 AND NOT EXISTS(SELECT 1 FROM tasks WHERE node=? AND status='running') RETURNING body`, id, node, node))
}
func (s *Store) Next(node string) (*engine.Task, error) {
	return readClaim(s.DB.QueryRow(`UPDATE tasks SET status='running', body=json_set(body,'$.status','running')
 WHERE id=(SELECT id FROM tasks WHERE node=? AND status='pending' ORDER BY created,id LIMIT 1)
 AND status='pending' AND NOT EXISTS(SELECT 1 FROM tasks WHERE node=? AND status='running') RETURNING body`, node, node))
}
func readClaim(row *sql.Row) (*engine.Task, error) {
	var body string
	err := row.Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t engine.Task
	if err = json.Unmarshal([]byte(body), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Completion preserves the server's original node, request and timestamp. Late
// results resolve interrupted tasks; repeated terminal results are acknowledged
// without replacing the first result (agents retry after a lost response).
func (s *Store) Complete(t engine.Task) error {
	if t.Status != "succeeded" && t.Status != "failed" {
		return errors.New("invalid completion status")
	}
	result, err := json.Marshal(t.Result)
	if err != nil {
		return err
	}
	updated, err := s.DB.Exec(`UPDATE tasks SET status=?, body=json_set(body,'$.status',?,'$.result',json(?),'$.error',?)
 WHERE id=? AND node=? AND status IN ('running','interrupted')`, t.Status, t.Status, string(result), t.Error, t.ID, t.Node)
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if count == 1 {
		return nil
	}
	original, err := s.Task(t.ID)
	if err != nil || original.Node != t.Node {
		return errors.New("task ownership mismatch")
	}
	if original.Status == "succeeded" || original.Status == "failed" {
		return nil
	}
	return errors.New("task has not been dispatched")
}
func (s *Store) Busy(node string) bool {
	var count int
	err := s.DB.QueryRow("SELECT count(*) FROM tasks WHERE node=? AND status IN ('pending','running')", node).Scan(&count)
	return err != nil || count > 0
}
func (s *Store) Prune() error {
	_, err := s.DB.Exec("DELETE FROM tasks WHERE created < ? AND status NOT IN ('pending','running')", time.Now().Add(-30*24*time.Hour).UTC().Format(time.RFC3339Nano))
	return err
}

// Remove only old scan payloads in SQL, without allocating/parsing each tree in
// Go or saving stale snapshots back over concurrently changing task records.
func (s *Store) CompactScans() error {
	_, err := s.DB.Exec(`UPDATE tasks SET body=json_remove(body,'$.result')
 WHERE id IN (SELECT id FROM tasks WHERE status='succeeded'
 AND json_extract(body,'$.request.kind')='scan' AND json_type(body,'$.result') IS NOT NULL
 AND json_type(body,'$.result')!='null' ORDER BY created DESC,id DESC LIMIT -1 OFFSET 10)`)
	return err
}
func (s *Store) ExpireTasks() error {
	_, err := s.DB.Exec(`UPDATE tasks SET status='interrupted', body=json_set(body,'$.status','interrupted','$.error',?)
 WHERE status IN ('pending','running') AND created < ?`,
		"task timed out; execution state may be unknown, inspect node before retrying", time.Now().Add(-3*time.Minute).UTC().Format(time.RFC3339Nano))
	return err
}
