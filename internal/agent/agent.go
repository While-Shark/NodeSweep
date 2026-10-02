package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/While-Shark/NodeSweep/internal/engine"
	"github.com/While-Shark/NodeSweep/internal/hub"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func Run(ctx context.Context, address, node, token string, e *engine.Engine) error {
	endpoint, err := pollURL(address)
	if err != nil {
		return err
	}
	if node == "" || len(node) > 64 || len(token) < 32 || len(token) > 256 || strings.ContainsAny(token, " \t\r\n") {
		return errors.New("invalid node credentials")
	}
	client := agentClient()
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
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Node-ID", node)
		resp, err := client.Do(req)
		if err == nil {
			var next *engine.Task
			if resp.StatusCode == 200 {
				next, err = readTask(resp.Body, node)
			} else {
				err = fmt.Errorf("hub HTTP %d", resp.StatusCode)
			}
			resp.Body.Close()
			if err == nil {
				pending = nil
				if next != nil && !busy {
					busy = true
					t := *next
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
