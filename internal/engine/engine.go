package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Engine struct {
	inspectOpen func() (map[[2]uint64]bool, error)
	Roots       []string
	ScanRoots   []string
	PanelRoots  []string
	mu          sync.Mutex
	plans       map[string]Plan
}

func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func New(roots, scanRoots []string) *Engine {
	return &Engine{Roots: roots, ScanRoots: scanRoots, plans: map[string]Plan{}, inspectOpen: openFiles}
}

// Paths are administrator-controlled on the agent. Requests cannot extend them.
func allowed(path string, roots []string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("absolute path required")
	}
	p := filepath.Clean(path)
	for _, r := range roots {
		r = filepath.Clean(r)
		rel, e := filepath.Rel(r, p)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
			return p, nil
		}
	}
	return "", errors.New("path is outside the agent allowlist")
}
func noSymlinks(p string) error {
	current := "/"
	for _, part := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		i, e := os.Lstat(current)
		if e != nil {
			return e
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink paths are not allowed")
		}
	}
	return nil
}
func (e *Engine) Run(ctx context.Context, r Request) (any, error) {
	e.mu.Lock()
	defer e.mu.Unlock() // Bound disk I/O to one operation per agent.
	if err := ValidateRequest(r); err != nil {
		return nil, err
	}
	switch r.Kind {
	case "scan":
		return e.scan(ctx, r.Path)
	case "preview":
		return e.preview(ctx, r.Rule)
	case "execute":
		return e.execute(ctx, r.PlanID)
	case "detect":
		return e.detect(), nil
	default:
		return nil, fmt.Errorf("unknown operation %q", r.Kind)
	}
}
func ValidateRule(r Rule) error {
	if !filepath.IsAbs(r.Root) || len(r.Root) > 4096 || strings.ContainsRune(r.Root, 0) {
		return errors.New("rule root must be an absolute path, max 4096 characters")
	}
	if filepath.Clean(r.Root) == "/" {
		return errors.New("filesystem root cannot be a cleanup root")
	}

	if len(r.Scheme) > 100 {
		return errors.New("scheme name too long")
	}
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 100 {
		return errors.New("rule name required (max 100 characters)")
	}
	if r.KeepDays < 1 || r.KeepDays > 3650 {
		return errors.New("keepDays must be 1–3650")
	}
	if len(r.Patterns) == 0 || len(r.Patterns) > 20 || len(r.Excludes) > 50 {
		return errors.New("provide 1–20 filename patterns and at most 50 exclusions")
	}
	for _, p := range append(append([]string{}, r.Patterns...), r.Excludes...) {
		if p == "" || len(p) > 255 || strings.ContainsRune(p, 0) {
			return errors.New("filename patterns must contain 1–255 characters")
		}
		if strings.Contains(p, "/") {
			return errors.New("patterns match filenames, not paths")
		}
		if _, err := filepath.Match(p, ""); err != nil {
			return err
		}
	}
	return nil
}
func match(name string, patterns []string) bool {
	for _, p := range patterns {
		ok, _ := filepath.Match(p, name)
		if ok {
			return true
		}
	}
	return false
}
