package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAuditBoundedRetentionAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.DB.Exec(`INSERT INTO audit_events(at,body) VALUES(?,?)`, time.Now().Add(-31*24*time.Hour).UTC().Format(time.RFC3339Nano), `{}`)
	for i := 0; i < 1001; i++ {
		id, err := s.BeginAudit(AuditEvent{Actor: "operator", Role: "operator", Action: "tasks.post"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 1000 {
			if err = s.FinishAudit(id, 403); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	events, err := s.Audit()
	if err != nil || len(events) != 1000 || events[0].Status != 403 || events[999].Status != 0 {
		t.Fatal(len(events), err)
	}
}
