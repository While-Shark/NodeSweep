package engine

import "time"

type Disk struct {
	Path       string `json:"path"`
	Total      uint64 `json:"total"`
	Available  uint64 `json:"available"`
	Inodes     uint64 `json:"inodes"`
	FreeInodes uint64 `json:"freeInodes"`
}
type Metrics struct {
	CPUAvailable    *bool     `json:"cpuAvailable,omitempty"`
	Partial         bool      `json:"partial,omitempty"`
	Host            string    `json:"host"`
	At              time.Time `json:"at"`
	MemoryTotal     uint64    `json:"memoryTotal"`
	MemoryAvailable uint64    `json:"memoryAvailable"`
	Load            string    `json:"load"`
	Uptime          string    `json:"uptime"`
	Disks           []Disk    `json:"disks"`
	CPU             float64   `json:"cpu"`
}
