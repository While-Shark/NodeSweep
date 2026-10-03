package engine

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotationReadsOnlySafeHints(t *testing.T) {
	root := t.TempDir()
	write := func(path, data string) {
		t.Helper()
		p := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("etc/docker/daemon.json", `{"log-driver":"json-file","log-opts":{"max-size":"10m","max-file":"3","env":"SECRET_TOKEN"},"authorization-plugins":["password-secret"]}`)
	write("etc/logrotate.conf", "include /secret-config\ndaily\nrotate 7\npostrotate\ncurl password-secret | sh\nrotate 0\nendscript\ncompress\n")
	write("etc/systemd/journald.conf", "[Other]\nSystemMaxUse=SECRET_TOKEN\n[Journal]\nSystemMaxUse=500M\nMaxRetentionSec=14day\nForwardToSyslog=yes\n")
	report := rotationChecks(context.Background(), root)
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "SECRET_TOKEN") || strings.Contains(string(raw), "password-secret") || strings.Contains(string(raw), "secret-config") {
		t.Fatal("configuration leaked", string(raw))
	}
	found := map[string]map[string]string{}
	for _, s := range report.Sources {
		found[s.Path] = s.Settings
	}
	if found["/etc/logrotate.conf"]["rotate"] != "7" || found["/etc/docker/daemon.json"]["max-file"] != "3" || found["/etc/systemd/journald.conf"]["SystemMaxUse"] != "500M" {
		t.Fatal(found)
	}
	if err := os.Symlink(filepath.Join(root, "etc/docker/daemon.json"), filepath.Join(root, "etc/logrotate.d")); err != nil {
		t.Fatal(err)
	}
	if data, ok := rotationFile(root, "/etc/logrotate.d"); ok || data != nil {
		t.Fatal("link followed")
	}
	fifo := filepath.Join(root, "etc/pipe")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := rotationFile(root, "/etc/pipe"); ok {
		t.Fatal("pipe accepted")
	}
	write("etc/large", strings.Repeat("x", (64<<10)+1))
	if _, ok := rotationFile(root, "/etc/large"); ok {
		t.Fatal("oversized accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := rotationChecks(ctx, root); !r.Partial || len(r.Sources) != 0 {
		t.Fatal("cancellation ignored", r)
	}
}
func TestRotationInputBoundsAndUnknowns(t *testing.T) {
	for _, data := range []string{`{"log-driver":"custom-secret","log-opts":{"max-size":"SECRET_TOKEN","max-file":3}}`, `invalid`} {
		out := rotationSettings("docker", []byte(data))
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "SECRET_TOKEN") {
			t.Fatal(string(raw))
		}
		if out["max-file"] != "" {
			t.Fatal("numeric Docker option accepted")
		}
	}
	root := t.TempDir()
	dir := filepath.Join(root, "etc/logrotate.d")
	os.MkdirAll(dir, 0700)
	for i := 0; i < 40; i++ {
		os.WriteFile(filepath.Join(dir, ID()), []byte("daily\nrotate 7\n"), 0600)
	}
	report := rotationChecks(context.Background(), root)
	count := 0
	for _, s := range report.Sources {
		if strings.HasPrefix(s.Path, "/etc/logrotate.d/") {
			count++
		}
	}
	raw, _ := json.Marshal(report)
	if !report.Partial || count > 16 || len(raw) > 32<<10 {
		t.Fatal("unbounded", count, len(raw))
	}
}
