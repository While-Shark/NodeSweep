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
	endpoint, err := validatedEndpoint(address, node, token)
	if err != nil {
		return err
	}
	client := agentClient()
	sampler := engine.Sampler{}
	done := make(chan engine.Task, 1)
	var worker sync.WaitGroup
	defer worker.Wait()
	var pending *engine.Task
	busy := false
	var activeID, activeKind string
	var cancelActive context.CancelFunc
	var progressMu sync.Mutex
	var progress *engine.ScanProgress
	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-done:
			pending = &t
			busy = false
			activeID = ""
			activeKind = ""
			if cancelActive != nil {
				cancelActive()
				cancelActive = nil
			}
			progressMu.Lock()
			progress = nil
			progressMu.Unlock()
		default:
		}
		progressMu.Lock()
		snapshot := progress
		progressMu.Unlock()
		body, err := json.Marshal(hub.Poll{LogChecks: true, ScanControl: true, TaskID: activeID, Progress: snapshot, Node: node, Metrics: sampler.ReadContext(ctx), Roots: e.Roots, ScanRoots: e.ScanRoots, Result: pending, Busy: busy})
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
			var response pollResponse
			if resp.StatusCode == 200 {
				response, err = readResponse(resp.Body, node)
				next = response.Task
			} else {
				err = fmt.Errorf("hub HTTP %d", resp.StatusCode)
			}
			resp.Body.Close()
			if err == nil {
				pending = nil
				if response.Cancel != "" {
					if response.Cancel == activeID && activeKind == "scan" && cancelActive != nil {
						cancelActive()
					} else {
						log.Print("ignored cancellation for a different or non-scan task")
					}
				}
				if next != nil && !busy {
					busy = true
					t := *next
					activeID = t.ID
					activeKind = t.Request.Kind
					work, cancel := context.WithTimeout(ctx, 90*time.Second)
					cancelActive = cancel
					if t.Request.Kind == "scan" {
						work = engine.WithScanProgress(work, func(update engine.ScanProgress) { progressMu.Lock(); progress = &update; progressMu.Unlock() })
					}
					worker.Add(1)
					go func() {
						defer worker.Done()
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

// ValidateSettings checks connection syntax locally without contacting the hub.
func ValidateSettings(address, node, token string) error {
	_, err := validatedEndpoint(address, node, token)
	return err
}
func validatedEndpoint(address, node, token string) (string, error) {
	endpoint, err := pollURL(address)
	if err != nil {
		return "", err
	}
	if node == "" || node == "local" || len(node) > 64 || len(token) < 32 || len(token) > 256 || strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("invalid node credentials")
	}
	return endpoint, nil
}
