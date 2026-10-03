package engine

import (
	"strings"
	"time"
)

// Lumberjack rotates name.log into name-2006-01-02T15-04-05.000.log.
// The preset glob narrows candidates; the parser also validates calendar dates.
const timestampLogPattern = "*-????-??-??T??-??-??.???.log"
const timestampLogLayout = "2006-01-02T15-04-05.000"

func archived(name string) bool {
	if strings.Contains(name, "-json.log.") || strings.Contains(name, ".journal") {
		return false
	}
	if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".xz") || strings.HasSuffix(name, ".bz2") {
		return true
	}
	if strings.HasSuffix(name, ".log") {
		stem := strings.TrimSuffix(name, ".log")
		start := len(stem) - len(timestampLogLayout)
		if start < 2 || stem[start-1] != '-' {
			return false
		}
		stamp := stem[start:]
		parsed, err := time.Parse(timestampLogLayout, stamp)
		return err == nil && parsed.Format(timestampLogLayout) == stamp
	}
	idx := strings.LastIndex(name, ".log.")
	if idx < 0 {
		return false
	}
	suffix := name[idx+5:]
	hasDigit := false
	for _, r := range suffix {
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else if r != '-' {
			return false
		}
	}
	return hasDigit
}
