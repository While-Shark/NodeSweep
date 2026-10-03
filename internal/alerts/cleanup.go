package alerts

import (
	"context"
	"encoding/json"
	"time"
)

// CheckCleanup records at most ten previously unreported terminal execution
// failures per pass. Claim before sending: restarting must not replay outbound
// notifications. A crash may leave delivery unknown; the task record remains.
func (s *Service) CheckCleanup(ctx context.Context, c Settings, now time.Time) error {
	if !c.Enabled || !c.CleanupFailures {
		return nil
	}
	cutoff := now.Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	rows, err := s.DB.Query(`SELECT t.id,t.node,COALESCE(json_extract(n.body,'$.name'),t.node)
 FROM tasks t LEFT JOIN nodes n ON n.id=t.node
 WHERE t.status IN ('failed','interrupted') AND t.created>=?
 AND json_extract(t.body,'$.request.kind')='execute'
 AND COALESCE(json_extract(t.body,'$.cleanupAlerted'),0)=0
 ORDER BY t.created,t.id LIMIT 10`, cutoff)
	if err != nil {
		return err
	}
	events := []Event{}
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.Task, &e.Node, &e.Name); err != nil {
			rows.Close()
			return err
		}
		events = append(events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range events {
		if err = ctx.Err(); err != nil {
			return err
		}
		e.Kind = "cleanup_failure"
		e.At = now
		e.Delivery = "not_configured"
		if s.Webhook != "" {
			e.Delivery = "interrupted"
		}
		body, _ := json.Marshal(e)
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		claim, err := tx.Exec(`UPDATE tasks SET body=json_set(body,'$.cleanupAlerted',json('true')) WHERE id=? AND status IN ('failed','interrupted') AND json_extract(body,'$.request.kind')='execute' AND COALESCE(json_extract(body,'$.cleanupAlerted'),0)=0`, e.Task)
		if err != nil {
			tx.Rollback()
			return err
		}
		count, err := claim.RowsAffected()
		if err != nil {
			tx.Rollback()
			return err
		}
		if count == 0 {
			tx.Rollback()
			continue
		}
		result, err := tx.Exec(`INSERT INTO alert_events(body) VALUES(?)`, string(body))
		if err != nil {
			tx.Rollback()
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err = tx.Exec(`DELETE FROM alert_events WHERE id NOT IN (SELECT id FROM alert_events ORDER BY id DESC LIMIT 100)`); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		if s.Webhook != "" {
			e.Delivery = "sent"
			if err = sendFormat(ctx, s.Webhook, s.Format, e); err != nil {
				e.Delivery = "failed"
			}
			body, _ = json.Marshal(e)
			if _, err = s.DB.Exec(`UPDATE alert_events SET body=? WHERE id=?`, string(body), id); err != nil {
				return err
			}
		}
	}
	return nil
}
