package store

import (
	"encoding/json"
	"errors"
	"github.com/While-Shark/NodeSweep/internal/engine"
)

// CancelScan preserves ownership and refuses destructive operation kinds.
func (s *Store) CancelScan(id string) (engine.Task, error) {
	var body string
	err := s.DB.QueryRow(`UPDATE tasks SET
 body=json_set(body,'$.cancelRequested',json('true'),'$.status',CASE status WHEN 'pending' THEN 'failed' ELSE status END,
 '$.error',CASE status WHEN 'pending' THEN 'scan cancelled before dispatch' ELSE coalesce(json_extract(body,'$.error'),'') END),
 status=CASE status WHEN 'pending' THEN 'failed' ELSE status END
 WHERE id=? AND status IN ('pending','running') AND json_extract(body,'$.request.kind')='scan' RETURNING body`, id).Scan(&body)
	var task engine.Task
	if err != nil {
		return task, errors.New("only pending or running scans can be cancelled")
	}
	err = json.Unmarshal([]byte(body), &task)
	return task, err
}
func (s *Store) ScanCancellation(node string) string {
	var id string
	_ = s.DB.QueryRow(`SELECT id FROM tasks WHERE node=? AND status='running' AND json_extract(body,'$.request.kind')='scan' AND json_extract(body,'$.cancelRequested')=1 LIMIT 1`, node).Scan(&id)
	return id
}
func (s *Store) ScanProgress(id, node string, progress engine.ScanProgress) error {
	body, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	updated, err := s.DB.Exec(`UPDATE tasks SET body=json_set(body,'$.progress',json(?))
 WHERE id=? AND node=? AND status='running' AND json_extract(body,'$.request.kind')='scan'
 AND coalesce(json_extract(body,'$.progress.visited'),0)<=?`, string(body), id, node, progress.Visited)
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		original, err := s.Task(id)
		if err != nil || original.Node != node || original.Request.Kind != "scan" {
			return errors.New("scan progress ownership mismatch")
		}
	}
	return nil
}
