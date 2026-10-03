package engine

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func managedLogRoots() []string {
	roots := []string{"/var/log/journal", "/run/log/journal", "/var/log/audit", "/var/lib/docker/containers"}
	if data, ok := rotationFile("/", "/etc/docker/daemon.json"); ok {
		var config struct {
			Root string `json:"data-root"`
		}
		if json.Unmarshal(data, &config) == nil && filepath.IsAbs(config.Root) && len(config.Root) <= 4096 && !strings.ContainsRune(config.Root, 0) {
			roots = append(roots, filepath.Join(config.Root, "containers"))
		}
	}
	return roots
}
func managedLogPath(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		root = filepath.Clean(root)
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}
