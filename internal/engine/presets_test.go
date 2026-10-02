package engine

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestPanelBaseOnlyAcceptsLiteralPaths(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{"#!/bin/bash\nBASE_DIR=/data\nORIGINAL_PASSWORD=secret", "/data"},
		{"BASE_DIR=\"/data/panel disk\" # installation\n", "/data/panel disk"},
		{"BASE_DIR='/srv'", "/srv"},
		{"BASE_DIR=$(touch /tmp/unsafe)", ""},
		{"BASE_DIR=\"$HOME/panel\"", ""},
		{"BASE_DIR=/srv; touch /tmp/unsafe", ""},
		{"BASE_DIR=/", ""},
		{"BASE_DIR=directory", ""},
	} {
		if got := panelBase(tc.data); got != tc.want {
			t.Errorf("panelBase(%q)=%q, want %q", tc.data, got, tc.want)
		}
	}
}
func TestNginxLogsIgnoreDynamicAndCommentedDirectives(t *testing.T) {
	data := `# access_log /wrong/site.log;
server { access_log /data/logs/access.log main; error_log "/data/logs/error.log" warn;
 access_log syslog:server=unix:/tmp/site_total.sock;
 access_log off;
 access_log /data/$host.log;
 error_log /relative.log;
 access_log /data/logs/[host].log;
}`
	logs := nginxLogs(data)
	if strings.Join(logs, ",") != "/data/logs/access.log,/data/logs/error.log" {
		t.Fatalf("unexpected logs: %v", logs)
	}
}
func TestDetectCustomPanelsAndDeduplicate(t *testing.T) {
	fsys := fstest.MapFS{
		"usr/local/bin/1pctl":                {Data: []byte("BASE_DIR='/srv/panels'\n")},
		"usr/bin/1pctl":                      {Data: []byte("BASE_DIR='/srv/panels'\n")},
		"srv/panels/1panel/log/current.log":  {},
		"data/bt/logs/current.log":           {},
		"data/bt/vhost/nginx/site.conf":      {Data: []byte("access_log /data/site-logs/site.log;\n")},
		"data/bt/vhost/nginx/duplicate.conf": {Data: []byte("access_log /data/site-logs/site.log;\n")},
		"data/site-logs/site.log":            {},
	}
	presets := detectPresets(fsys, []string{"/data/bt"})
	customPanel, customSite := 0, 0
	for _, p := range presets {
		if err := ValidateRule(p.Rule); err != nil {
			t.Fatalf("invalid generated rule: %v", err)
		}
		if p.Rule.Root == "/srv/panels/1panel/log" {
			customPanel++
			if !p.Detected {
				t.Fatal("custom 1Panel not detected")
			}
		}
		if p.Rule.Root == "/data/site-logs" {
			customSite++
			if !p.Detected || !match("site.log.1.gz", p.Rule.Patterns) || match("other.log.1.gz", p.Rule.Patterns) || !match("site.log", p.Rule.Excludes) {
				t.Fatal("incorrect targeted site rule")
			}
		}
	}
	if customPanel != 1 || customSite != 1 {
		t.Fatalf("custom panel=%d, site=%d", customPanel, customSite)
	}
}
func TestDetectionRejectsOversizedConfiguration(t *testing.T) {
	fsys := fstest.MapFS{"usr/local/bin/1pctl": {Data: []byte("BASE_DIR=/data\n" + strings.Repeat("x", 256<<10))}}
	for _, p := range detectPresets(fsys, nil) {
		if p.Rule.Root == "/data/1panel/log" {
			t.Fatal("oversized configuration accepted")
		}
	}
}
