package engine

import "time"

type Rule struct {
	Scheme   string   `json:"scheme"`
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Root     string   `json:"root"`
	Patterns []string `json:"patterns"`
	Excludes []string `json:"excludes"`
	KeepDays int      `json:"keepDays"`
}
type Candidate struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Inode    uint64 `json:"inode"`
	Device   uint64 `json:"device"`
}
type Decision struct {
	Path    string `json:"path"`
	Reason  string `json:"reason"`
	Pattern string `json:"pattern,omitempty"`
}
type Review struct {
	Counts   map[string]int `json:"counts"`
	Examples []Decision     `json:"examples"`
}
type Plan struct {
	Review  Review      `json:"review"`
	ID      string      `json:"id"`
	Rule    Rule        `json:"rule"`
	Created time.Time   `json:"created"`
	Files   []Candidate `json:"files"`
	Bytes   int64       `json:"bytes"`
}
type CleanupItem struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Bytes  int64  `json:"bytes"`
}
type CleanupResult struct {
	Root         string        `json:"root"`
	RuleName     string        `json:"ruleName"`
	Items        []CleanupItem `json:"items"`
	Planned      int           `json:"planned"`
	PlannedBytes int64         `json:"plannedBytes"`
	Started      time.Time     `json:"started"`
	Finished     time.Time     `json:"finished"`
	Deleted      int           `json:"deleted"`
	Bytes        int64         `json:"bytes"`
	Skipped      []string      `json:"skipped"`
}
type Entry struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Bytes     int64    `json:"bytes"`
	Directory bool     `json:"directory"`
	Children  []*Entry `json:"children,omitempty"`
}
type Scan struct {
	Tree      *Entry    `json:"tree"`
	Files     int       `json:"files"`
	Skipped   int       `json:"skipped"`
	Truncated bool      `json:"truncated"`
	At        time.Time `json:"at"`
}
type Request struct {
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Rule   Rule   `json:"rule,omitempty"`
	PlanID string `json:"planId,omitempty"`
}
type Task struct {
	ID      string    `json:"id"`
	Node    string    `json:"node"`
	Request Request   `json:"request"`
	Status  string    `json:"status"`
	Result  any       `json:"result,omitempty"`
	Error   string    `json:"error,omitempty"`
	Created time.Time `json:"created"`
}
