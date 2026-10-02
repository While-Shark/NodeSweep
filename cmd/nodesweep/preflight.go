package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/agent"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"io"
	"path/filepath"
	"runtime"
)

func validateConfig(c Config) error {
	if c.Mode != "standalone" && c.Mode != "hub" && c.Mode != "agent" {
		return errors.New("mode must be standalone, hub or agent")
	}
	if len(c.CleanupRoots) > 64 || len(c.ScanRoots) > 64 {
		return errors.New("at most 64 cleanup and scan roots allowed")
	}
	for _, p := range append(append([]string{}, c.ScanRoots...), c.CleanupRoots...) {
		if !filepath.IsAbs(p) || len(p) > 4096 {
			return errors.New("allowlist roots must be absolute")
		}
	}
	for _, p := range c.CleanupRoots {
		p = filepath.Clean(p)
		if p == "/" || p == "/etc" || p == "/proc" || p == "/sys" || p == "/dev" {
			return fmt.Errorf("unsafe cleanup root: %s", p)
		}
	}
	if len(c.PanelRoots) > 16 {
		return errors.New("at most 16 panelRoots allowed")
	}
	for _, p := range c.PanelRoots {
		if !filepath.IsAbs(p) || filepath.Clean(p) == "/" {
			return errors.New("panelRoots must be absolute panel installation directories")
		}
	}

	if c.Mode == "agent" {
		return agent.ValidateSettings(c.Hub, c.Node, c.Token)
	}
	if !validCredential(c.AdminToken) {
		return errors.New("adminToken must contain at least 32 characters without outer whitespace")
	}
	return nil
}

// Local checks never start HTTP, open the database, send credentials or delete files.
func preflight(c Config, w io.Writer) error {
	checks := []engine.EnvironmentCheck{}
	if c.Mode != "hub" {
		checks = engine.New(c.CleanupRoots, c.ScanRoots).CheckEnvironment()
	}
	report := struct {
		Mode         string                    `json:"mode"`
		Architecture string                    `json:"architecture"`
		Checks       []engine.EnvironmentCheck `json:"checks"`
	}{c.Mode, runtime.GOARCH, checks}
	if err := json.NewEncoder(w).Encode(report); err != nil {
		return err
	}
	for _, check := range checks {
		if !check.OK {
			return errors.New("local preflight failed; inspect checks before starting")
		}
	}
	return nil
}
