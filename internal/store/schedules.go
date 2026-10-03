package store

import (
	"encoding/json"
	"errors"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"time"
)

// Rule is an immutable snapshot. Editing a saved rule cannot expand consent.
type Schedule struct {
	ID      string      `json:"id"`
	Node    string      `json:"node"`
	Rule    engine.Rule `json:"rule"`
	Hours   int         `json:"hours"`
	Enabled bool        `json:"enabled"`
	Next    time.Time   `json:"next"`
	Task    string      `json:"task,omitempty"`
	Phase   string      `json:"phase,omitempty"`
	Outcome string      `json:"outcome,omitempty"`
}

func (s *Store) Schedules() ([]Schedule, error) {
	rows, err := s.DB.Query("SELECT body FROM schedules ORDER BY id LIMIT 32")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		var raw string
		var v Schedule
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Schedule(id string) (Schedule, error) {
	var v Schedule
	var raw string
	err := s.DB.QueryRow("SELECT body FROM schedules WHERE id=?", id).Scan(&raw)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal([]byte(raw), &v)
	return v, err
}
func (s *Store) SaveSchedule(v Schedule) error {
	if !engine.ValidID(v.ID) || v.Hours < 1 || v.Hours > 720 {
		return errors.New("invalid schedule")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO schedules(id,body) SELECT ?,? WHERE (SELECT count(*) FROM schedules)<32 OR EXISTS(SELECT 1 FROM schedules WHERE id=?) ON CONFLICT(id) DO UPDATE SET body=excluded.body`, v.ID, string(raw), v.ID)
	if err != nil {
		return err
	}
	// The bounded insert may be refused. Do not pretend it was saved.
	var found string
	return s.DB.QueryRow("SELECT id FROM schedules WHERE id=?", v.ID).Scan(&found)
}
func (s *Store) DeleteSchedule(id string) error {
	_, err := s.DB.Exec("DELETE FROM schedules WHERE id=?", id)
	return err
}

// Save task and scheduler transition atomically: no task can exist without a
// consumed tick. Audit must already have been persisted by the caller.
func (s *Store) ScheduleTask(v Schedule, t engine.Task) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	task, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	updated, err := tx.Exec("UPDATE schedules SET body=? WHERE id=? AND json_extract(body,'$.enabled')=1", string(body), v.ID)
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("schedule no longer enabled")
	}
	if _, err = tx.Exec("INSERT INTO tasks VALUES(?,?,?,?,?)", t.ID, t.Node, t.Status, t.Created.UTC().Format(time.RFC3339Nano), string(task)); err != nil {
		return err
	}
	return tx.Commit()
}
