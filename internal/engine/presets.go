package engine

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Preset struct {
	Name     string `json:"name"`
	Detected bool   `json:"detected"`
	Rule     Rule   `json:"rule"`
	Note     string `json:"note"`
}

// Detection proposes rules; it never extends the node's cleanup allowlist.
func Detect() []Preset { return detectPresets(os.DirFS("/"), nil) }

func (e *Engine) detect() []Preset { return detectPresets(os.DirFS("/"), e.PanelRoots) }

func detectPresets(fsys fs.FS, panelRoots []string) []Preset {
	out := []Preset{}
	seen := map[string]bool{}
	add := func(name, path, note string, patterns, excludes []string) {
		path = filepath.Clean(path)
		key := path + "\x00" + strings.Join(patterns, "\x00")
		if seen[key] {
			return
		}
		seen[key] = true
		info, err := fs.Stat(fsys, strings.TrimPrefix(path, "/"))
		out = append(out, Preset{name, err == nil && info.IsDir(), Rule{ID: ID(), Scheme: name, Name: name, Root: path, Patterns: patterns, Excludes: excludes, KeepDays: 14}, note})
	}
	for _, x := range []struct{ name, path string }{
		{"Linux 归档日志", "/var/log"}, {"Nginx 归档日志", "/var/log/nginx"},
		{"宝塔网站日志", "/www/wwwlogs"}, {"宝塔面板日志", "/www/server/panel/logs"},
		{"1Panel 日志", "/opt/1panel/log"},
	} {
		patterns := []string{"*.log.*", "*.gz"}
		if x.name == "1Panel 日志" {
			patterns = append(patterns, timestampLogPattern)
		}
		add(x.name, x.path, "仅处理过期归档；清理前须在节点白名单允许该目录", patterns, []string{"journal", "audit"})
	}
	for _, path := range []string{"/usr/local/bin/1pctl", "/usr/bin/1pctl"} {
		data, ok := readDetectionFile(fsys, path)
		if !ok {
			continue
		}
		base := panelBase(data)
		if base != "" {
			add("1Panel 日志", filepath.Join(base, "1panel/log"), "从 1pctl 的静态 BASE_DIR 识别；未执行脚本", []string{"*.log.*", "*.gz", timestampLogPattern}, nil)
		}
	}
	roots := append([]string{"/www/server/panel"}, panelRoots...)
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			continue
		}
		if root != "/www/server/panel" {
			add("宝塔面板日志", filepath.Join(root, "logs"), "从节点 panelRoots 配置识别", []string{"*.log.*", "*.gz"}, nil)
		}
		dir := filepath.Join(root, "vhost/nginx")
		f, err := fsys.Open(strings.TrimPrefix(dir, "/"))
		if err != nil {
			continue
		}
		rd, ok := f.(fs.ReadDirFile)
		if !ok {
			f.Close()
			continue
		}
		entries, _ := rd.ReadDir(128)
		f.Close()
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".conf") {
				continue
			}
			data, ok := readDetectionFile(fsys, filepath.Join(dir, ent.Name()))
			if !ok {
				continue
			}
			for _, log := range nginxLogs(data) {
				name := filepath.Base(log)
				add("宝塔站点归档", filepath.Dir(log), "从 Nginx 静态日志指令识别；保留当前日志 "+name, []string{name + ".*"}, []string{name})
				if len(out) >= 100 {
					return out
				}
			}
		}
	}
	return out
}

// Bound each configuration read and refuse nonregular files (including pipes).
func readDetectionFile(fsys fs.FS, path string) (string, bool) {
	info, err := fs.Stat(fsys, strings.TrimPrefix(path, "/"))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return "", false
	}
	f, err := fsys.Open(strings.TrimPrefix(path, "/"))
	if err != nil {
		return "", false
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	return string(data), err == nil && len(data) <= 256<<10
}

var baseAssignment = regexp.MustCompile(`(?m)^\s*BASE_DIR=("[^"\n]*"|'[^'\n]*'|[^\s#;]+)\s*(?:#.*)?$`)
var literalPath = regexp.MustCompile(`^/[a-zA-Z0-9_./ -]+$`)
var logDirective = regexp.MustCompile(`(?m)(?:^|[;{}\n])\s*(?:access_log|error_log)\s+("[^"\n]*"|'[^'\n]*'|[^\s;]+)[^;\n]*;`)
var literalLogName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func unquoteLiteral(value string) string {
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
		return value[1 : len(value)-1]
	}
	return value
}
func panelBase(data string) string {
	m := baseAssignment.FindStringSubmatch(data)
	if len(m) == 0 {
		return ""
	}
	path := unquoteLiteral(m[1])
	if !literalPath.MatchString(path) || filepath.Clean(path) == "/" {
		return ""
	}
	return filepath.Clean(path)
}
func nginxLogs(data string) []string {
	// Strip comments outside quotes. No include expansion or variable evaluation.
	var clean strings.Builder
	var quote rune
	comment := false
	for _, c := range data {
		if c == '\n' {
			comment = false
			quote = 0
			clean.WriteRune(c)
			continue
		}
		if comment {
			continue
		}
		if c == '#' && quote == 0 {
			comment = true
			continue
		}
		if c == '\'' || c == '"' {
			if quote == 0 {
				quote = c
			} else if quote == c {
				quote = 0
			}
		}
		clean.WriteRune(c)
		if c == ';' && quote == 0 {
			clean.WriteRune('\n')
		}
	}
	out := []string{}
	for _, m := range logDirective.FindAllStringSubmatch(clean.String(), -1) {
		path := unquoteLiteral(m[1])
		if !literalPath.MatchString(path) || !literalLogName.MatchString(filepath.Base(path)) || !strings.Contains(filepath.Base(path), ".log") {
			continue
		}
		path = filepath.Clean(path)
		if filepath.Dir(path) == "/" {
			continue
		}
		out = append(out, path)
	}
	return out
}
