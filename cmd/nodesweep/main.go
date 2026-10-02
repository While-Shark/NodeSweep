package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/agent"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/hub"
	"github.com/While-Shark/NodeSweep/internal/store"
	"github.com/While-Shark/NodeSweep/web"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	Mode         string   `json:"mode"`
	Listen       string   `json:"listen"`
	Data         string   `json:"data"`
	AdminToken   string   `json:"adminToken"`
	Hub          string   `json:"hub"`
	Node         string   `json:"node"`
	Token        string   `json:"token"`
	CleanupRoots []string `json:"cleanupRoots"`
	ScanRoots    []string `json:"scanRoots"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	path := flag.String("config", "config.json", "configuration path")
	init := flag.Bool("init", false, "write secure standalone configuration")
	flag.Parse()
	if *init {
		c := Config{Mode: "standalone", Listen: "127.0.0.1:9780", Data: "data/nodesweep.db", AdminToken: engine.ID() + engine.ID(), CleanupRoots: []string{"/var/log"}, ScanRoots: []string{"/"}}
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(b)
		if err == nil {
			fmt.Println("Created configuration. Read adminToken from this file to sign in.")
		}
		return err
	}
	b, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	var c Config
	if err = json.Unmarshal(b, &c); err != nil {
		return err
	}
	if c.Mode != "standalone" && c.Mode != "hub" && c.Mode != "agent" {
		return errors.New("mode must be standalone, hub or agent")
	}
	for _, p := range append(append([]string{}, c.ScanRoots...), c.CleanupRoots...) {
		if !filepath.IsAbs(p) {
			return errors.New("allowlist roots must be absolute")
		}
	}
	for _, p := range c.CleanupRoots {
		p = filepath.Clean(p)
		if p == "/" || p == "/etc" || p == "/proc" || p == "/sys" || p == "/dev" {
			return fmt.Errorf("unsafe cleanup root: %s", p)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	e := engine.New(c.CleanupRoots, c.ScanRoots)
	if c.Mode == "agent" {
		if c.Node == "" || len(c.Token) < 32 {
			return errors.New("node and strong token required")
		}
		return agent.Run(ctx, c.Hub, c.Node, c.Token, e)
	}
	if len(c.AdminToken) < 32 || strings.TrimSpace(c.AdminToken) != c.AdminToken {
		return errors.New("adminToken must contain at least 32 characters without outer whitespace")
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:9780"
	}
	if c.Data == "" {
		c.Data = "data/nodesweep.db"
	}
	s, err := store.Open(c.Data)
	if err != nil {
		return err
	}
	defer s.DB.Close()
	h := &hub.Hub{Store: s, Token: c.AdminToken}
	if c.Mode == "standalone" {
		h.Engine = e
		go h.LocalMetrics(ctx)
	}
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		return err
	}
	server := &http.Server{Addr: c.Listen, Handler: h.Handler(assets), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 40 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.ExpireTasks(); err != nil {
					log.Print(err)
				}
				if err := s.CompactScans(); err != nil {
					log.Print(err)
				}
				if err := s.Prune(); err != nil {
					log.Print(err)
				}
			}
		}
	}()
	log.Printf("NodeSweep %s listening on %s", c.Mode, c.Listen)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
