package engine

import "os"

type Preset struct {
	Name     string `json:"name"`
	Detected bool   `json:"detected"`
	Rule     Rule   `json:"rule"`
	Note     string `json:"note"`
}

func Detect() []Preset {
	out := []Preset{}
	for _, x := range []struct{ name, path, note string }{
		{"Linux 归档日志", "/var/log", "仅处理过期归档；不删除 journal 或活跃日志"},
		{"Nginx 归档日志", "/var/log/nginx", "需在 Agent 清理白名单中允许此目录"},
		{"宝塔网站日志", "/www/wwwlogs", "常见默认路径；自定义位置请手动填写"},
		{"宝塔面板日志", "/www/server/panel/logs", "仅清理匹配的历史归档"},
		{"1Panel 日志", "/opt/1panel/log", "常见默认路径；自定义安装路径请手动填写"},
	} {
		i, err := os.Stat(x.path)
		out = append(out, Preset{x.name, err == nil && i.IsDir(), Rule{ID: ID(), Name: x.name, Root: x.path, Patterns: []string{"*.log.*", "*.gz"}, Excludes: []string{"journal", "audit"}, KeepDays: 14}, x.note})
	}
	return out
}
