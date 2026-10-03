package store

import (
	"github.com/While-Shark/NodeSweep/internal/engine"
	"path/filepath"
	"testing"
	"time"
)

func TestMetricReceiptRetentionAndAggregation(t *testing.T) {
	s := testStore(t)
	s.AddNode(Node{ID: "one"}, "")
	s.AddNode(Node{ID: "two"}, "")
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	m := engine.Metrics{CPU: 20, MemoryTotal: 100, MemoryAvailable: 25, Disks: []engine.Disk{{Path: "/", Total: 100, Available: 40, Inodes: 100, FreeInodes: 90}}}
	if err := s.recordMetrics("one", m, now.Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"one", "two"} {
		if err := s.recordMetrics(n, m, now.Add(-2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	m.CPU = 80
	m.MemoryAvailable = 75
	s.recordMetrics("one", m, now.Add(-time.Minute))
	m.CPU = 99
	s.recordMetrics("one", m, now.Add(-time.Minute))
	points, err := s.MetricsHistory("one", now, false)
	if err != nil || len(points) != 1 {
		t.Fatal(points, err)
	}
	p := points[0]
	if p.Samples != 2 || *p.CPU != 50 || *p.Memory != 50 || *p.Disks[0].Used != 60 || *p.Disks[0].Inodes < 9.9 {
		t.Fatal(p)
	}
	var count int
	s.DB.QueryRow("SELECT count(*) FROM metric_samples").Scan(&count)
	if count != 3 {
		t.Fatal("expired or duplicate retained", count)
	}
	// A revoked node cannot create orphan samples; revocation erases its history.
	if err = s.DeleteNode("one"); err != nil {
		t.Fatal(err)
	}
	s.recordMetrics("one", m, now)
	points, err = s.MetricsHistory("one", now, true)
	if err != nil || len(points) != 0 {
		t.Fatal(points, err)
	}
}
func TestHistoryBoundsAndUnavailableValues(t *testing.T) {
	s := testStore(t)
	s.AddNode(Node{ID: "one"}, "")
	now := time.Now()
	m := engine.Metrics{CPU: 101, MemoryTotal: 1, MemoryAvailable: 2}
	for i := 0; i < 30; i++ {
		m.Disks = append(m.Disks, engine.Disk{Path: string(rune('A' + i)), Total: 0})
	}
	if err := s.recordMetrics("one", m, now); err != nil {
		t.Fatal(err)
	}
	points, err := s.MetricsHistory("one", now, false)
	if err != nil || len(points) != 1 {
		t.Fatal(err)
	}
	if points[0].CPU != nil || points[0].Memory != nil || len(points[0].Disks) != 16 || points[0].Disks[0].Used != nil {
		t.Fatal("invalid value/cap", points)
	}
}

func TestHistoryGlobalCapAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddNode(Node{ID: "one"}, ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	// Seed many nodes/minutes inside the retention window to exercise the global
	// cap rather than the seven-day expiration branch.
	_, err = s.DB.Exec(`WITH RECURSIVE samples(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM samples WHERE n<20001)
 INSERT INTO metric_samples SELECT 'fixture-'||n,?, '{}' FROM samples`, now.Add(-time.Minute).Unix()/60)
	if err != nil {
		t.Fatal(err)
	}
	m := engine.Metrics{CPU: 37, At: now.Add(-30 * 24 * time.Hour)}
	if err = s.recordMetrics("one", m, now); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.QueryRow("SELECT count(*) FROM metric_samples").Scan(&count)
	if count != 20000 {
		t.Fatal(count)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	points, err := s.MetricsHistory("one", now, false)
	if err != nil || len(points) != 1 || *points[0].CPU != 37 {
		t.Fatal(points, err)
	}
}
