package engine

import (
	"context"
	"errors"
)

// Budgets are local configuration, never supplied in hub task requests.
type ScanBudget struct {
	Entries     int `json:"entries"`
	Seconds     int `json:"seconds"`
	Depth       int `json:"depth"`
	TreeBytes   int `json:"treeBytes"`
	PauseMillis int `json:"pauseMillis"`
}

func (b ScanBudget) defaults() ScanBudget {
	if b.Entries == 0 {
		b.Entries = 100000
	}
	if b.Seconds == 0 {
		b.Seconds = 60
	}
	if b.Depth == 0 {
		b.Depth = 64
	}
	if b.TreeBytes == 0 {
		b.TreeBytes = 8 << 20
	}
	if b.PauseMillis == 0 {
		b.PauseMillis = 5
	}
	return b
}
func ValidateScanBudget(b ScanBudget) error {
	b = b.defaults()
	if b.Entries < 1 || b.Entries > 100000 || b.Seconds < 1 || b.Seconds > 80 || b.Depth < 1 || b.Depth > 64 || b.TreeBytes < 1024 || b.TreeBytes > 16<<20 || b.PauseMillis < 1 || b.PauseMillis > 100 {
		return errors.New("invalid scan budget")
	}
	return nil
}

type ScanProgress struct {
	Visited       int   `json:"visited"`
	Files         int   `json:"files"`
	Bytes         int64 `json:"bytes"`
	Limit         int   `json:"limit"`
	ElapsedMillis int64 `json:"elapsedMillis"`
}
type scanObserver struct{}

// WithScanProgress provides aggregate counts only, without filenames or credentials.
func WithScanProgress(ctx context.Context, observer func(ScanProgress)) context.Context {
	return context.WithValue(ctx, scanObserver{}, observer)
}
