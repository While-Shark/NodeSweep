package agent

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/While-Shark/NodeSweep/internal/engine"
)

func pollURL(address string) (string, error) {
	u, err := url.Parse(address)
	if err != nil {
		return "", errors.New("invalid hub URL")
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
		return "", errors.New("hub URL must not contain credentials, query, fragment or encoded path")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return "", errors.New("agent requires HTTPS, except loopback development")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/agent/poll"
	return u.String(), nil
}
func agentClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func readTask(body io.Reader, node string) (*engine.Task, error) {
	data, err := io.ReadAll(io.LimitReader(body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("hub response exceeds limit")
	}
	var msg struct {
		Task *engine.Task `json:"task"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&msg); err != nil {
		return nil, errors.New("invalid hub response")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid hub response")
	}
	if msg.Task != nil {
		if msg.Task.Node != node || !engine.ValidID(msg.Task.ID) || msg.Task.Status != "running" {
			return nil, errors.New("task identity mismatch")
		}
		if err := engine.ValidateRequest(msg.Task.Request); err != nil {
			return nil, err
		}
	}
	return msg.Task, nil
}
