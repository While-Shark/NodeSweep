package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalPreflightDoesNotWriteOrExposeCredentials(t *testing.T) {
	dir := t.TempDir()
	c := Config{Mode: "hub", AdminToken: strings.Repeat("secret", 8), Hub: "https://private.example", Data: filepath.Join(dir, "new.db")}
	if err := validateConfig(c); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := preflight(c, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "private.example") {
		t.Fatal("sensitive output")
	}
	if _, err := os.Stat(c.Data); !os.IsNotExist(err) {
		t.Fatal("preflight created database", err)
	}
	c = Config{Mode: "agent", Node: "node", Token: strings.Repeat("x", 64), Hub: "http://remote.example"}
	if validateConfig(c) == nil {
		t.Fatal("unsafe remote HTTP accepted")
	}
	c.Hub = "https://example.com"
	c.CleanupRoots = []string{"/etc/../etc"}
	if validateConfig(c) == nil {
		t.Fatal("unsafe root accepted")
	}
	c.CleanupRoots = nil
	if err := validateConfig(c); err != nil {
		t.Fatal(err)
	}
	c.ScanRoots = []string{filepath.Join(dir, "missing")}
	out.Reset()
	if preflight(c, &out) == nil {
		t.Fatal("missing directory passed")
	}
}
