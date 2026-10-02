package alerts

import (
	"context"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/store"
)

func TestAlertsTransitionsCooldownAndRestart(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	service, err := New(db.DB, "")
	if err != nil {
		t.Fatal(err)
	}
	config, err := service.Settings()
	if err != nil || config.Enabled {
		t.Fatal(config, err)
	}
	config.Enabled = true
	config.CooldownSeconds = 60
	if err = service.Save(config); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	node := store.Node{ID: "node", Name: "fixture", LastSeen: now, Metrics: engine.Metrics{Disks: []engine.Disk{{Path: "/", Total: 100, Available: 10, Inodes: 100, FreeInodes: 50}}}}
	check := func(at time.Time) {
		t.Helper()
		node.LastSeen = at
		if err = service.Check(context.Background(), []store.Node{node}, config, at); err != nil {
			t.Fatal(err)
		}
	}
	check(now)
	check(now.Add(10 * time.Second))
	events, err := service.Events()
	if err != nil || len(events) != 1 || events[0].Kind != "disk" || events[0].Resolved {
		t.Fatal(events, err)
	}
	service, err = New(db.DB, "")
	if err != nil {
		t.Fatal(err)
	}
	check(now.Add(20 * time.Second))
	events, _ = service.Events()
	if len(events) != 1 {
		t.Fatal("restart duplicated notification")
	}
	node.Metrics.Disks[0].Available = 50
	check(now.Add(30 * time.Second))
	events, _ = service.Events()
	if len(events) != 2 || !events[0].Resolved {
		t.Fatal(events)
	}
	node.Metrics.Disks[0].Available = 10
	check(now.Add(40 * time.Second))
	node.LastSeen = now.Add(40 * time.Second)
	if err = service.Check(context.Background(), []store.Node{node}, config, now.Add(120*time.Second)); err != nil {
		t.Fatal(err)
	}
	events, _ = service.Events()
	if events[0].Kind != "offline" || events[0].Resolved {
		t.Fatal(events)
	}
	if len(events) != 4 {
		t.Fatalf("offline fabricated disk recovery: %+v", events)
	}
	config.DiskPercent = 101
	if service.Save(config) == nil {
		t.Fatal("invalid threshold accepted")
	}
	config.DiskPercent = 85
	for i := 0; i < 105; i++ {
		node.LastSeen = now.Add(time.Duration(i+1000) * time.Minute)
		check(node.LastSeen)
	}
	events, _ = service.Events()
	if len(events) != 100 {
		t.Fatal("unbounded history", len(events))
	}
}
func TestWebhookDestinationPolicy(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "0.1.2.3", "192.0.2.1", "198.18.0.1", "::1", "::ffff:10.1.2.3", "fc00::1", "2001:db8::1", "64:ff9b::a00:1", "2002:a00:1::"} {
		if publicIP(netip.MustParseAddr(value)) {
			t.Fatal("private/reserved destination allowed", value)
		}
	}
	for _, value := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(value)) {
			t.Fatal(value)
		}
	}
	for _, value := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/#token", "file:///etc/passwd"} {
		if validateURL(value) == nil {
			t.Fatal(value)
		}
	}
	if validateURL("https://example.com/hooks/token") != nil {
		t.Fatal("valid webhook rejected")
	}
}
