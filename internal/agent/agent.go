package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/hub"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

func Run(ctx context.Context, address, node, token string, e *engine.Engine) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return errors.New("agent requires HTTPS, except loopback development")
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	sampler := engine.Sampler{}
	done := make(chan engine.Task, 1)
	var worker sync.WaitGroup
	defer worker.Wait()
	var pending *engine.Task
	busy := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-done:
			pending = &t
			busy = false
		default:
		}
		body, err := json.Marshal(hub.Poll{Node: node, Metrics: sampler.Read(), Roots: e.Roots, ScanRoots: e.ScanRoots, Result: pending, Busy: busy})
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(address, "/")+"/agent/poll", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Node-ID", node)
		resp, err := client.Do(req)
		if err == nil {
			var msg struct {
				Task *engine.Task `json:"task"`
			}
			if resp.StatusCode == 200 {
				err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&msg)
			} else {
				err = fmt.Errorf("hub HTTP %d", resp.StatusCode)
			}
			resp.Body.Close()
			if err == nil {
				pending = nil
				if msg.Task != nil && !busy {
					busy = true
					t := *msg.Task
					worker.Add(1)
					go func() {
						defer worker.Done()
						work, cancel := context.WithTimeout(ctx, 90*time.Second)
						defer cancel()
						result, runErr := e.Run(work, t.Request)
						t.Result = result
						t.Status = "succeeded"
						if runErr != nil {
							t.Status = "failed"
							t.Error = runErr.Error()
						}
						done <- t
					}()
				}
			}
		}
		if err != nil {
			log.Printf("node poll: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}
