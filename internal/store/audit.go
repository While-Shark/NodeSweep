package store

import (
	"encoding/json"
	"time"
)

type AuditEvent struct {
	ID     int64     `json:"id"`
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Role   string    `json:"role"`
	Action string    `json:"action"`
	Target string    `json:"target,omitempty"`
	Status int       `json:"status"`
}

func (s *Store) BeginAudit(e AuditEvent) (int64, error) {
	e.At = time.Now().UTC()
	raw, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO audit_events(at,body) VALUES(?,?)`, e.At.Format(time.RFC3339Nano), string(raw))
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`DELETE FROM audit_events WHERE at<? OR id NOT IN(SELECT id FROM audit_events ORDER BY id DESC LIMIT 1000)`, e.At.Add(-30*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}
func (s *Store) FinishAudit(id int64, status int) error {
	_, err := s.DB.Exec(`UPDATE audit_events SET body=json_set(body,'$.status',?) WHERE id=?`, status, id)
	return err
}
func (s *Store) Audit() ([]AuditEvent, error) {
	rows, err := s.DB.Query(`SELECT id,body FROM audit_events ORDER BY id DESC LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var id int64
		var raw string
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, err
		}
		e.ID = id
		out = append(out, e)
	}
	return out, rows.Err()
}
