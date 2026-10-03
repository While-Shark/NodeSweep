package store

import (
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/While-Shark/NodeSweep/internal/engine"
)

// Only numeric utilization and mount paths are retained; no hostname, tokens,
// process data or agent-provided timestamps enter history.
type MetricDisk struct {
	Path   string   `json:"path"`
	Used   *float64 `json:"used"`
	Inodes *float64 `json:"inodes"`
}
type MetricPoint struct {
	At      time.Time    `json:"at"`
	Samples int          `json:"samples"`
	CPU     *float64     `json:"cpu"`
	Memory  *float64     `json:"memory"`
	Disks   []MetricDisk `json:"disks"`
}

func percent(total, available uint64) *float64 {
	if total == 0 || available > total {
		return nil
	}
	v := 100 * (1 - float64(available)/float64(total))
	return &v
}
func (s *Store) recordMetrics(node string, m engine.Metrics, at time.Time) error {
	p := MetricPoint{At: at.UTC().Truncate(time.Minute), Samples: 1, Disks: []MetricDisk{}, Memory: percent(m.MemoryTotal, m.MemoryAvailable)}
	if !math.IsNaN(m.CPU) && !math.IsInf(m.CPU, 0) && m.CPU >= 0 && m.CPU <= 100 {
		v := m.CPU
		p.CPU = &v
	}
	seen := map[string]bool{}
	estimatedBytes := 512
	for _, d := range m.Disks {
		if len(p.Disks) >= 16 {
			break
		}
		if d.Path == "" || len(d.Path) > 256 || seen[d.Path] {
			continue
		}
		if estimatedBytes+96+6*len(d.Path) > 4096 {
			continue
		}
		estimatedBytes += 96 + 6*len(d.Path)
		seen[d.Path] = true
		p.Disks = append(p.Disks, MetricDisk{d.Path, percent(d.Total, d.Available), percent(d.Inodes, d.FreeInodes)})
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// The first accepted sample per receipt minute wins. Concurrent heartbeat
	// retries cannot inflate the history or overwrite it with forged timestamps.
	result, err := tx.Exec(`INSERT OR IGNORE INTO metric_samples(node,minute,body)
 SELECT ?,?,? WHERE EXISTS(SELECT 1 FROM nodes WHERE id=?)`, node, p.At.Unix()/60, string(b), node)
	if err != nil {
		return err
	}
	added, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if added > 0 {
		if _, err = tx.Exec(`DELETE FROM metric_samples WHERE minute<=?`, p.At.Add(-7*24*time.Hour).Unix()/60); err != nil {
			return err
		}
		// A global cap protects hubs with many nodes; the oldest samples expire first.
		if _, err = tx.Exec(`DELETE FROM metric_samples WHERE rowid IN (SELECT rowid FROM metric_samples ORDER BY minute DESC LIMIT -1 OFFSET 20000)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type mean struct {
	sum   float64
	count int
}

func (m *mean) add(v *float64) {
	if v != nil {
		m.sum += *v
		m.count++
	}
}
func (m mean) value() *float64 {
	if m.count == 0 {
		return nil
	}
	v := m.sum / float64(m.count)
	return &v
}

type diskMean struct{ used, inodes mean }
type metricMean struct {
	samples     int
	cpu, memory mean
	disks       map[string]*diskMean
}

// MetricsHistory returns at most 169 aggregates. Missing samples stay missing;
// offline periods are never filled with zero utilization.
func (s *Store) MetricsHistory(node string, now time.Time, week bool) ([]MetricPoint, error) {
	duration, bucket := 24*time.Hour, 15*time.Minute
	if week {
		duration, bucket = 7*24*time.Hour, time.Hour
	}
	rows, err := s.DB.Query(`SELECT minute,body FROM metric_samples WHERE node=? AND minute>? AND minute<=? ORDER BY minute LIMIT 10080`, node, now.Add(-duration).Unix()/60, now.Unix()/60)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := map[int64]*metricMean{}
	order := []int64{}
	for rows.Next() {
		var minute int64
		var raw string
		if err = rows.Scan(&minute, &raw); err != nil {
			return nil, err
		}
		var p MetricPoint
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		key := minute / (int64(bucket / time.Minute))
		g := groups[key]
		if g == nil {
			g = &metricMean{disks: map[string]*diskMean{}}
			groups[key] = g
			order = append(order, key)
		}
		g.samples++
		g.cpu.add(p.CPU)
		g.memory.add(p.Memory)
		for _, d := range p.Disks {
			a := g.disks[d.Path]
			if a == nil {
				if len(g.disks) >= 16 {
					continue
				}
				a = &diskMean{}
				g.disks[d.Path] = a
			}
			a.used.add(d.Used)
			a.inodes.add(d.Inodes)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := []MetricPoint{}
	for _, key := range order {
		g := groups[key]
		p := MetricPoint{At: time.Unix(key*int64(bucket/time.Second), 0).UTC(), Samples: g.samples, CPU: g.cpu.value(), Memory: g.memory.value(), Disks: []MetricDisk{}}
		paths := []string{}
		for path := range g.disks {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			a := g.disks[path]
			p.Disks = append(p.Disks, MetricDisk{path, a.used.value(), a.inodes.value()})
		}
		out = append(out, p)
	}
	return out, nil
}
