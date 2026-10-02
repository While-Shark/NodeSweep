package engine

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Disk struct {
	Path       string `json:"path"`
	Total      uint64 `json:"total"`
	Available  uint64 `json:"available"`
	Inodes     uint64 `json:"inodes"`
	FreeInodes uint64 `json:"freeInodes"`
}
type Metrics struct {
	Host            string    `json:"host"`
	At              time.Time `json:"at"`
	MemoryTotal     uint64    `json:"memoryTotal"`
	MemoryAvailable uint64    `json:"memoryAvailable"`
	Load            string    `json:"load"`
	Uptime          string    `json:"uptime"`
	Disks           []Disk    `json:"disks"`
	CPU             float64   `json:"cpu"`
}
type Sampler struct{ total, idle uint64 }

func (s *Sampler) Read() Metrics {
	m := Metrics{At: time.Now(), Disks: []Disk{}}
	m.Host, _ = os.Hostname()
	b, _ := os.ReadFile("/proc/meminfo")
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		if f[0] == "MemTotal:" {
			m.MemoryTotal = v * 1024
		}
		if f[0] == "MemAvailable:" {
			m.MemoryAvailable = v * 1024
		}
	}
	b, _ = os.ReadFile("/proc/loadavg")
	f := strings.Fields(string(b))
	if len(f) >= 3 {
		m.Load = strings.Join(f[:3], " / ")
	}
	b, _ = os.ReadFile("/proc/uptime")
	f = strings.Fields(string(b))
	if len(f) > 0 {
		m.Uptime = f[0]
	}
	b, _ = os.ReadFile("/proc/stat")
	line := strings.Split(string(b), "\n")[0]
	f = strings.Fields(line)
	var total, idle uint64
	for j := 1; j < len(f) && j <= 8; j++ {
		v, _ := strconv.ParseUint(f[j], 10, 64)
		total += v
		if j == 4 || j == 5 {
			idle += v
		}
	}
	if s.total > 0 && total > s.total {
		m.CPU = 100 * (1 - float64(idle-s.idle)/float64(total-s.total))
	}
	s.total = total
	s.idle = idle
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return m
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	seen := map[uint64]bool{}
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 {
			continue
		}
		if f[2] != "ext4" && f[2] != "xfs" && f[2] != "btrfs" && f[2] != "overlay" && f[2] != "zfs" && f[2] != "ext3" {
			continue
		}
		p := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\134`, `\`).Replace(f[1])
		var st syscall.Stat_t
		if syscall.Stat(p, &st) != nil || seen[st.Dev] {
			continue
		}
		var fs syscall.Statfs_t
		if syscall.Statfs(p, &fs) != nil {
			continue
		}
		seen[st.Dev] = true
		m.Disks = append(m.Disks, Disk{p, fs.Blocks * uint64(fs.Bsize), fs.Bavail * uint64(fs.Bsize), fs.Files, fs.Ffree})
	}
	return m
}
