package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/agent"
	"github.com/While-Shark/NodeSweep/internal/alerts"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/hub"
	"github.com/While-Shark/NodeSweep/internal/store"
	"github.com/While-Shark/NodeSweep/web"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	WebhookURL   string   `json:"webhookURL,omitempty"`
	Mode         string   `json:"mode"`
	Listen       string   `json:"listen"`
	Data         string   `json:"data"`
	AdminToken   string   `json:"adminToken"`
	Hub          string   `json:"hub"`
	Node         string   `json:"node"`
	Token        string   `json:"token"`
	CleanupRoots []string `json:"cleanupRoots"`
	ScanRoots    []string `json:"scanRoots"`
	PanelRoots   []string `json:"panelRoots,omitempty"`
}

var version = "dev"
var commit = "unknown"
var builtAt = "unknown"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	path := flag.String("config", "config.json", "configuration path")
	init := flag.Bool("init", false, "write secure standalone configuration")
	check := flag.Bool("check", false, "validate local configuration and directory access without starting services")
	showVersion := flag.Bool("version", false, "print build version")
	flag.Parse()
	if *check && (*init || *showVersion) {
		return errors.New("check cannot be combined with init or version")
	}
	if *showVersion {
		fmt.Printf("NodeSweep %s commit=%s built=%s\n", version, commit, builtAt)
		return nil
	}
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
	c, err := readConfig(*path)
	if err != nil {
		return err
	}
	if err := validateConfig(c); err != nil {
		return err
	}
	if *check {
		return preflight(c, os.Stdout)
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancelWork := context.WithCancel(signalCtx)
	defer cancelWork()
	defer stop()
	e := engine.New(c.CleanupRoots, c.ScanRoots)
	e.PanelRoots = c.PanelRoots
	if c.Mode == "agent" {
		if c.Node == "" || !validCredential(c.Token) {
			return errors.New("node and strong token required")
		}
		return agent.Run(ctx, c.Hub, c.Node, c.Token, e)
	}
	if !validCredential(c.AdminToken) {
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
	notifications, err := alerts.New(s.DB, c.WebhookURL)
	if err != nil {
		return err
	}
	h := &hub.Hub{Store: s, Token: c.AdminToken, Context: ctx, Alerts: notifications}
	var background sync.WaitGroup
	defer func() { cancelWork(); h.Wait(); background.Wait() }()
	background.Add(1)
	go func() { defer background.Done(); notifications.Run(ctx, s.Nodes) }()
	if c.Mode == "standalone" {
		h.Engine = e
		background.Add(1)
		go func() { defer background.Done(); h.LocalMetrics(ctx) }()
	}
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		return err
	}
	server := &http.Server{Addr: c.Listen, Handler: h.Handler(assets), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 40 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	serverFinished := make(chan struct{})
	go func() {
		defer close(serverFinished)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	background.Add(1)
	go func() {
		defer background.Done()
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
	cancelWork()
	<-serverFinished
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
