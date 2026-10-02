package engine

// EnvironmentCheck contains no credentials or URLs. Paths are local allowlist entries.
type EnvironmentCheck struct {
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
	OK   bool   `json:"ok"`
	Note string `json:"note"`
}

func (e *Engine) CheckEnvironment() []EnvironmentCheck {
	checks := []EnvironmentCheck{}
	for _, list := range []struct {
		kind  string
		paths []string
	}{{"cleanup_root", e.Roots}, {"scan_root", e.ScanRoots}} {
		for _, path := range list.paths {
			root, err := openDirectory(path)
			check := EnvironmentCheck{Kind: list.kind, Path: path, OK: err == nil, Note: "directory opens without symlinks"}
			if err != nil {
				check.Note = "directory missing, inaccessible or contains a symlink"
			} else {
				_ = root.Close()
			}
			checks = append(checks, check)
		}
	}
	if len(e.Roots) > 0 {
		_, err := e.inspectOpen()
		checks = append(checks, EnvironmentCheck{Kind: "process_visibility", OK: err == nil, Note: "checks visible process descriptors only; cannot prove host PID namespace visibility"})
	}
	return checks
}
