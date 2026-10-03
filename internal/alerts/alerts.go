package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/While-Shark/NodeSweep/internal/store"
)

type Settings struct {
	CleanupFailures bool `json:"cleanupFailures"`
	Enabled         bool `json:"enabled"`
	DiskPercent     int  `json:"diskPercent"`
	InodePercent    int  `json:"inodePercent"`
	OfflineSeconds  int  `json:"offlineSeconds"`
	CooldownSeconds int  `json:"cooldownSeconds"`
}
type Event struct {
	Task     string    `json:"task,omitempty"`
	ID       int64     `json:"id"`
	Node     string    `json:"node"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`
	Percent  float64   `json:"percent"`
	Resolved bool      `json:"resolved"`
	At       time.Time `json:"at"`
	Delivery string    `json:"delivery"`
}
type state struct {
	Active bool      `json:"active"`
	Sent   time.Time `json:"sent"`
}
type Service struct {
	DB      *sql.DB
	Webhook string
	Format  string
}

func New(db *sql.DB, webhook string) (*Service, error) {
	if err := validateURL(webhook); err != nil {
		return nil, err
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS alert_settings(id INTEGER PRIMARY KEY CHECK(id=1),body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS alert_state(key TEXT PRIMARY KEY,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS alert_events(id INTEGER PRIMARY KEY AUTOINCREMENT,body TEXT NOT NULL);`)
	return &Service{DB: db, Webhook: webhook}, err
}
func (s *Service) Settings() (Settings, error) {
	v := Settings{DiskPercent: 85, InodePercent: 85, OfflineSeconds: 60, CooldownSeconds: 1800}
	var raw string
	err := s.DB.QueryRow("SELECT body FROM alert_settings WHERE id=1").Scan(&raw)
	if err == sql.ErrNoRows {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal([]byte(raw), &v)
	return v, err
}
func (s *Service) Save(v Settings) error {
	if v.DiskPercent < 1 || v.DiskPercent > 100 || v.InodePercent < 1 || v.InodePercent > 100 || v.OfflineSeconds < 45 || v.OfflineSeconds > 86400 || v.CooldownSeconds < 60 || v.CooldownSeconds > 86400 {
		return errors.New("invalid alert thresholds")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("INSERT INTO alert_settings VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", string(b))
	return err
}
func (s *Service) Events() ([]Event, error) {
	rows, err := s.DB.Query("SELECT id,body FROM alert_events ORDER BY id DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var raw string
		var e Event
		var id int64
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, err
		}
		e.ID = id
		events = append(events, e)
	}
	return events, rows.Err()
}
func (s *Service) Run(ctx context.Context, nodes func() ([]store.Node, error)) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			settings, err := s.Settings()
			if err != nil {
				log.Print("alert settings unavailable")
				continue
			}
			if !settings.Enabled {
				continue
			}
			list, err := nodes()
			if err != nil {
				continue
			}
			if settings.CleanupFailures {
				failureCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				if err = s.CheckCleanup(failureCtx, settings, now); err != nil {
					log.Print("cleanup alert processing failed")
				}
				cancel()
			}
			if err = s.Check(ctx, list, settings, now); err != nil {
				log.Print("alert processing failed")
			}
		}
	}
}
func (s *Service) Check(ctx context.Context, nodes []store.Node, c Settings, now time.Time) error {
	seen := map[string]bool{}
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		offline := now.Sub(node.LastSeen) > time.Duration(c.OfflineSeconds)*time.Second
		if err := s.transition(ctx, node, "offline", "", 0, offline, c, now, seen); err != nil {
			return err
		}
		for _, d := range node.Metrics.Disks {
			if err := ctx.Err(); err != nil {
				return err
			}
			if offline {
				for _, kind := range []string{"disk", "inode"} {
					key, _ := json.Marshal([]string{node.ID, kind, d.Path})
					seen[string(key)] = true
				}
				continue
			}
			if d.Total > 0 && d.Available <= d.Total {
				p := 100 * (1 - float64(d.Available)/float64(d.Total))
				if err := s.transition(ctx, node, "disk", d.Path, p, !offline && p >= float64(c.DiskPercent), c, now, seen); err != nil {
					return err
				}
			}
			if d.Inodes > 0 && d.FreeInodes <= d.Inodes {
				p := 100 * (1 - float64(d.FreeInodes)/float64(d.Inodes))
				if err := s.transition(ctx, node, "inode", d.Path, p, !offline && p >= float64(c.InodePercent), c, now, seen); err != nil {
					return err
				}
			}
		}
	}
	// Revoke stale state for removed nodes/mounts without fabricating recovery metrics.
	rows, err := s.DB.Query("SELECT key FROM alert_state")
	if err != nil {
		return err
	}
	stale := []string{}
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			rows.Close()
			return err
		}
		if !seen[k] {
			stale = append(stale, k)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, key := range stale {
		if _, err = s.DB.Exec("DELETE FROM alert_state WHERE key=?", key); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) transition(ctx context.Context, n store.Node, kind, path string, percent float64, active bool, c Settings, now time.Time, seen map[string]bool) error {
	keyBytes, _ := json.Marshal([]string{n.ID, kind, path})
	key := string(keyBytes)
	seen[key] = true
	var prior state
	var raw string
	err := s.DB.QueryRow("SELECT body FROM alert_state WHERE key=?", key).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &prior); err != nil {
			return err
		}
	}
	notify := active && (!prior.Active || now.Sub(prior.Sent) >= time.Duration(c.CooldownSeconds)*time.Second) || !active && prior.Active
	if notify {
		e := Event{Node: n.ID, Name: n.Name, Kind: kind, Path: path, Percent: percent, Resolved: !active, At: now, Delivery: "not_configured"}
		if s.Webhook != "" {
			e.Delivery = "sent"
			if err = sendFormat(ctx, s.Webhook, s.Format, e); err != nil {
				e.Delivery = "failed"
			}
		}
		b, _ := json.Marshal(e)
		if _, err = s.DB.Exec("INSERT INTO alert_events(body) VALUES(?)", string(b)); err != nil {
			return err
		}
		if _, err = s.DB.Exec("DELETE FROM alert_events WHERE id NOT IN (SELECT id FROM alert_events ORDER BY id DESC LIMIT 100)"); err != nil {
			return err
		}
		prior.Sent = now
	}
	prior.Active = active
	b, _ := json.Marshal(prior)
	_, err = s.DB.Exec("INSERT INTO alert_state VALUES(?,?) ON CONFLICT(key) DO UPDATE SET body=excluded.body", key, string(b))
	return err
}
