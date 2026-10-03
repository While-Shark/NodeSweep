package engine

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RotationSource struct {
	Kind     string            `json:"kind"`
	Path     string            `json:"path"`
	Status   string            `json:"status"`
	Settings map[string]string `json:"settings"`
}
type RotationReport struct {
	At      time.Time        `json:"at"`
	Partial bool             `json:"partial"`
	Sources []RotationSource `json:"sources"`
}

// Pin parent directories and refuse links, pipes and devices before reading.
// Only fixed system config paths are inspected; requests supply no paths.
func rotationFile(base, path string) ([]byte, bool) {
	parent, err := openDirectory(filepath.Join(base, strings.TrimPrefix(filepath.Dir(path), "/")))
	if err != nil {
		return nil, false
	}
	defer parent.Close()
	fd, err := unix.Openat(parent.fd, filepath.Base(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, false
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	return data, err == nil && len(data) <= 64<<10
}

var rotationSize = regexp.MustCompile(`^[1-9][0-9]{0,10}[kKmMgGtTpPeE]?$`)
var dockerSize = regexp.MustCompile(`^[1-9][0-9]{0,10}[kKmMgG]$`)
var retentionTime = regexp.MustCompile(`^[0-9]{1,10}(?:us|ms|s|sec|seconds?|m|min|minutes?|h|hr|hours?|d|days?|w|weeks?|months?|years?)?$`)

// Static hints only: includes, environment, command line and running services
// are not evaluated. Script blocks are skipped and never executed or returned.
func rotationSettings(kind string, data []byte) map[string]string {
	out := map[string]string{}
	if kind == "docker" {
		var c struct {
			Driver  string                     `json:"log-driver"`
			Options map[string]json.RawMessage `json:"log-opts"`
		}
		if json.Unmarshal(data, &c) != nil {
			return out
		}
		switch c.Driver {
		case "local", "json-file", "journald", "syslog", "none":
			out["log-driver"] = c.Driver
		default:
			if c.Driver != "" {
				out["log-driver"] = "other"
			}
		}
		for _, key := range []string{"max-size", "max-file"} {
			var value string
			if json.Unmarshal(c.Options[key], &value) != nil {
				continue
			}
			if key == "max-size" && dockerSize.MatchString(value) {
				out[key] = value
			}
			if key == "max-file" {
				n, err := strconv.Atoi(value)
				if err == nil && n > 0 && n <= 100000 {
					out[key] = strconv.Itoa(n)
				}
			}
		}
		return out
	}
	script := false
	section := ""
	for i, line := range strings.Split(string(data), "\n") {
		if i >= 1024 {
			break
		}
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line == "" {
			continue
		}
		if kind == "journal" {
			if strings.HasPrefix(line, "[") {
				section = line
				continue
			}
			if section != "[Journal]" {
				continue
			}
			kv := strings.SplitN(line, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key, value := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			switch key {
			case "SystemMaxUse", "RuntimeMaxUse", "SystemKeepFree", "RuntimeKeepFree", "SystemMaxFileSize", "RuntimeMaxFileSize":
				if rotationSize.MatchString(value) {
					out[key] = value
				}
			case "MaxRetentionSec", "MaxFileSec":
				if retentionTime.MatchString(value) {
					out[key] = value
				}
			case "Storage":
				switch value {
				case "auto", "volatile", "persistent", "none":
					out[key] = value
				}
			}
			continue
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] == "endscript" {
			script = false
			continue
		}
		if script {
			continue
		}
		switch f[0] {
		case "prerotate", "postrotate", "firstaction", "lastaction", "preremove":
			script = true
			continue
		}
		switch f[0] {
		case "daily", "weekly", "monthly", "yearly", "hourly", "compress", "nocompress", "copytruncate", "nocopytruncate", "dateext", "nodateext":
			out[f[0]] = "present"
		case "rotate":
			if len(f) == 2 {
				n, err := strconv.Atoi(f[1])
				if err == nil && n >= 0 && n <= 100000 {
					out["rotate"] = strconv.Itoa(n)
				}
			}
		case "size", "maxsize", "minsize":
			if len(f) == 2 && rotationSize.MatchString(f[1]) {
				out[f[0]] = f[1]
			}
		case "include":
			out["include"] = "present"
		}
	}
	return out
}
func rotationChecks(parent context.Context, base string) RotationReport {
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	report := RotationReport{At: time.Now(), Sources: []RotationSource{}}
	type source struct{ kind, path string }
	sources := []source{{"logrotate", "/etc/logrotate.conf"}, {"docker", "/etc/docker/daemon.json"}}
	// Report sources independently; do not claim to compute effective overrides.
	for _, dir := range []string{"/etc/logrotate.d", "/usr/lib/systemd/journald.conf.d", "/usr/local/lib/systemd/journald.conf.d", "/run/systemd/journald.conf.d", "/etc/systemd/journald.conf.d"} {
		if ctx.Err() != nil {
			report.Partial = true
			break
		}
		parent, err := openDirectory(filepath.Join(base, strings.TrimPrefix(dir, "/")))
		if err != nil {
			report.Partial = true
			continue
		}
		f, err := parent.Open(".")
		if err != nil {
			parent.Close()
			report.Partial = true
			continue
		}
		entries, err := f.ReadDir(17)
		f.Close()
		parent.Close()
		if err != nil && err != io.EOF {
			report.Partial = true
		}
		if len(entries) > 16 {
			entries = entries[:16]
			report.Partial = true
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || len(entry.Name()) > 128 {
				continue
			}
			kind := "journal"
			if dir == "/etc/logrotate.d" {
				kind = "logrotate"
			} else if !strings.HasSuffix(entry.Name(), ".conf") {
				continue
			}
			sources = append(sources, source{kind, filepath.Join(dir, entry.Name())})
		}
	}
	for _, dir := range []string{"/usr/lib/systemd", "/usr/local/lib/systemd", "/run/systemd", "/etc/systemd"} {
		sources = append(sources, source{"journal", dir + "/journald.conf"})
	}
	total, cost := 0, 512
	for _, source := range sources {
		if ctx.Err() != nil || total >= 1<<20 {
			report.Partial = true
			break
		}
		data, ok := rotationFile(base, source.path)
		total += len(data)
		item := RotationSource{Kind: source.kind, Path: source.path, Status: "unavailable", Settings: map[string]string{}}
		if ok {
			item.Settings = rotationSettings(source.kind, data)
			item.Status = "observed"
			if len(item.Settings) == 0 {
				item.Status = "not_configured"
			}
			if strings.Count(string(data), "\n") >= 1024 {
				report.Partial = true
			}
		} else {
			report.Partial = true
		}
		raw, _ := json.Marshal(item)
		if cost+len(raw) > 32<<10 {
			report.Partial = true
			break
		}
		cost += len(raw) + 1
		report.Sources = append(report.Sources, item)
	}
	return report
}
