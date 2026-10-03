package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func metricFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{"meminfo": "MemTotal: 1024 kB\nMemAvailable: 512 kB\n", "loadavg": "0.1 0.2 0.3 1/100 1\n", "uptime": "120.0 100.0\n", "stat": "cpu 100 0 0 100 0 0 0 0\n", "mounts": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestSamplerWarmupAndCounterRegression(t *testing.T) {
	root := metricFixture(t)
	s := Sampler{procRoot: root}
	first := s.Read()
	if first.CPUAvailable == nil || *first.CPUAvailable || first.Partial || first.MemoryAvailable != 512*1024 {
		t.Fatal(first)
	}
	os.WriteFile(filepath.Join(root, "stat"), []byte("cpu 150 0 0 150 0 0 0 0\n"), 0600)
	second := s.Read()
	if !*second.CPUAvailable || second.CPU != 50 {
		t.Fatal(second)
	}
	os.WriteFile(filepath.Join(root, "stat"), []byte("cpu 10 0 0 10 0 0 0 0\n"), 0600)
	if next := s.Read(); *next.CPUAvailable {
		t.Fatal("regressing counters reported utilization", next)
	}
}
func TestSamplerLimitsMountChecksAndPayload(t *testing.T) {
	root := metricFixture(t)
	var mounts strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&mounts, "device /%d ext4 rw 0 0\n", i)
	}
	os.WriteFile(filepath.Join(root, "mounts"), []byte(mounts.String()), 0600)
	calls := 0
	s := Sampler{procRoot: root, probe: func(p string) (uint64, Disk, error) {
		calls++
		return uint64(calls), Disk{Path: p, Total: 100, Available: 50}, nil
	}}
	m := s.Read()
	raw, _ := json.Marshal(m)
	if calls != 128 || len(m.Disks) != 128 || !m.Partial || len(raw) > 32<<10 {
		t.Fatal(calls, len(m.Disks), m.Partial, len(raw))
	}
	// Long paths consume the conservative JSON budget before any stat call.
	long := "/" + strings.Repeat("a", 4095)
	os.WriteFile(filepath.Join(root, "mounts"), []byte("device "+long+" ext4 rw 0 0\ndevice "+long+"b ext4 rw 0 0\n"), 0600)
	calls = 0
	m = s.Read()
	raw, _ = json.Marshal(m)
	if calls != 1 || len(m.Disks) != 1 || !m.Partial || len(raw) > 32<<10 {
		t.Fatal(calls, len(raw))
	}
}
func TestSamplerCancellationAndBoundedProcInputs(t *testing.T) {
	root := metricFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	os.WriteFile(filepath.Join(root, "mounts"), []byte("device /a ext4 rw 0 0\ndevice /b ext4 rw 0 0\n"), 0600)
	calls := 0
	s := Sampler{procRoot: root, probe: func(p string) (uint64, Disk, error) { calls++; cancel(); return 1, Disk{Path: p}, nil }}
	m := s.ReadContext(ctx)
	if calls > 1 || !m.Partial {
		t.Fatal(calls, m)
	}
	calls = 0
	m = s.ReadContext(ctx)
	if calls != 0 || !m.Partial || *m.CPUAvailable {
		t.Fatal("cancelled sampling performed filesystem checks")
	}
	os.WriteFile(filepath.Join(root, "mounts"), nil, 0600)
	os.WriteFile(filepath.Join(root, "stat"), []byte("cpu 10 0 0 10 0 0 0 0\n"+strings.Repeat("cpu1 1 2 3 4\n", 100000)), 0600)
	s = Sampler{procRoot: root}
	m = s.Read()
	if m.Partial {
		t.Fatal("read beyond aggregate CPU line", m)
	}
	os.WriteFile(filepath.Join(root, "meminfo"), []byte(strings.Repeat("x", 100000)), 0600)
	m = s.Read()
	if !m.Partial || m.MemoryTotal != 0 {
		t.Fatal("oversized input accepted")
	}
	os.WriteFile(filepath.Join(root, "mounts"), []byte(strings.Repeat("device /tmp tmpfs rw 0 0\n", 5000)), 0600)
	os.WriteFile(filepath.Join(root, "meminfo"), nil, 0600)
	m = s.Read()
	if !m.Partial {
		t.Fatal("unbounded mount line traversal")
	}
}

func TestMissingMemoryIsNotReportedAsFullUsage(t *testing.T) {
	root := metricFixture(t)
	os.WriteFile(filepath.Join(root, "meminfo"), []byte("MemTotal: 1024 kB\n"), 0600)
	sampler := Sampler{procRoot: root}
	m := sampler.Read()
	if !m.Partial || m.MemoryTotal != 0 || m.MemoryAvailable != 0 {
		t.Fatal("missing available memory fabricated usage", m)
	}
}
