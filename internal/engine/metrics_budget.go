package engine

import (
	"bufio"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Sampler struct {
	total, idle uint64
	procRoot    string
	probe       func(string) (uint64, Disk, error)
}

func (s *Sampler) Read() Metrics { return s.ReadContext(context.Background()) }
func metricFile(path string, limit int64, firstLine bool) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := io.LimitReader(file, limit+1)
	var data []byte
	if firstLine {
		line, e := bufio.NewReader(reader).ReadString('\n')
		if e != nil && e != io.EOF {
			return "", e
		}
		data = []byte(line)
	} else {
		data, err = io.ReadAll(reader)
		if err != nil {
			return "", err
		}
	}
	if int64(len(data)) > limit {
		return "", errors.New("metric input budget exceeded")
	}
	return string(data), nil
}
func probeDisk(path string) (uint64, Disk, error) {
	var stat syscall.Stat_t
	var fs syscall.Statfs_t
	if err := syscall.Stat(path, &stat); err != nil {
		return 0, Disk{}, err
	}
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, Disk{}, err
	}
	if fs.Bsize <= 0 || fs.Blocks > math.MaxUint64/uint64(fs.Bsize) || fs.Bavail > fs.Blocks || fs.Ffree > fs.Files {
		return 0, Disk{}, errors.New("invalid filesystem metrics")
	}
	return stat.Dev, Disk{path, fs.Blocks * uint64(fs.Bsize), fs.Bavail * uint64(fs.Bsize), fs.Files, fs.Ffree}, nil
}

// Budgets are checked between filesystem calls. A kernel-blocked read/statfs
// cannot be interrupted safely; no unbounded goroutines are spawned to mask it.
func (s *Sampler) ReadContext(parent context.Context) Metrics {
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	available := false
	m := Metrics{At: time.Now(), Disks: []Disk{}, CPUAvailable: &available}
	if ctx.Err() != nil {
		m.Partial = true
		return m
	}
	m.Host, _ = os.Hostname()
	root := s.procRoot
	if root == "" {
		root = "/proc"
	}
	read := func(name string, limit int64, first bool) string {
		if ctx.Err() != nil {
			m.Partial = true
			return ""
		}
		data, err := metricFile(filepath.Join(root, name), limit, first)
		if err != nil {
			m.Partial = true
		}
		return data
	}
	foundTotal, foundAvailable := false, false
	for _, line := range strings.Split(read("meminfo", 64<<10, false), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "MemTotal:" && f[0] != "MemAvailable:" {
			continue
		}
		if len(f) != 3 || f[2] != "kB" {
			m.Partial = true
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil || v > math.MaxUint64/1024 {
			m.Partial = true
			continue
		}
		if f[0] == "MemTotal:" {
			foundTotal = true
			m.MemoryTotal = v * 1024
		} else {
			foundAvailable = true
			m.MemoryAvailable = v * 1024
		}
	}
	if !foundTotal || !foundAvailable {
		m.MemoryTotal = 0
		m.MemoryAvailable = 0
		m.Partial = true
	}
	if m.MemoryTotal == 0 {
		m.Partial = true
	}
	if m.MemoryAvailable > m.MemoryTotal {
		m.MemoryAvailable = 0
		m.MemoryTotal = 0
		m.Partial = true
	}
	f := strings.Fields(read("loadavg", 4096, true))
	if len(f) >= 3 {
		m.Load = strings.Join(f[:3], " / ")
		if len(m.Load) > 100 {
			m.Load = ""
			m.Partial = true
		}
	}
	f = strings.Fields(read("uptime", 4096, true))
	if len(f) > 0 {
		if len(f[0]) <= 100 {
			m.Uptime = f[0]
		} else {
			m.Partial = true
		}
	}
	f = strings.Fields(read("stat", 64<<10, true))
	var total, idle uint64
	valid := len(f) >= 6 && f[0] == "cpu"
	for j := 1; j < len(f) && j <= 8; j++ {
		v, err := strconv.ParseUint(f[j], 10, 64)
		if err != nil || v > math.MaxUint64-total {
			valid = false
			break
		}
		total += v
		if j == 4 || j == 5 {
			idle += v
		}
	}
	if valid && s.total > 0 && total > s.total && idle >= s.idle && idle-s.idle <= total-s.total {
		m.CPU = 100 * (1 - float64(idle-s.idle)/float64(total-s.total))
		available = true
	}
	if valid {
		s.total = total
		s.idle = idle
	} else {
		m.Partial = true
	}
	if ctx.Err() != nil {
		m.Partial = true
		return m
	}
	file, err := os.Open(filepath.Join(root, "mounts"))
	if err != nil {
		m.Partial = true
		return m
	}
	defer file.Close()
	limited := &io.LimitedReader{R: file, N: (1 << 20) + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	seen := map[uint64]bool{}
	attempts, lines, cost := 0, 0, 4096
	probe := s.probe
	if probe == nil {
		probe = probeDisk
	}
	for scanner.Scan() {
		lines++
		if ctx.Err() != nil || lines > 4096 {
			m.Partial = true
			break
		}
		f := strings.Fields(scanner.Text())
		if len(f) < 3 {
			continue
		}
		switch f[2] {
		case "ext4", "xfs", "btrfs", "overlay", "zfs", "ext3":
		default:
			continue
		}
		if attempts >= 128 {
			m.Partial = true
			break
		}
		attempts++
		path := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\134`, `\`).Replace(f[1])
		if !filepath.IsAbs(path) || len(path) > 4096 || cost+192+6*len(path) > 32<<10 {
			m.Partial = true
			continue
		}
		dev, disk, err := probe(path)
		if err != nil {
			m.Partial = true
			continue
		}
		if seen[dev] {
			continue
		}
		seen[dev] = true
		cost += 192 + 6*len(path)
		m.Disks = append(m.Disks, disk)
	}
	if scanner.Err() != nil || limited.N <= 0 || ctx.Err() != nil {
		m.Partial = true
	}
	return m
}
