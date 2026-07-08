package procmon

import (
	"fmt"
	"sync"
	"time"
)

// Snapshot is process CPU and memory use for the running binary.
type Snapshot struct {
	CPUPercent float64 `json:"cpu_percent"`
	RSSBytes   uint64  `json:"rss_bytes"`
	RSS        string  `json:"rss"`
}

// Monitor samples this process on each Snapshot call.
type Monitor struct {
	mu          sync.Mutex
	lastCPU     time.Duration
	lastAt      time.Time
	initialized bool
}

// New creates a process monitor for the current OS process.
func New() *Monitor { return &Monitor{} }

// Snapshot returns CPU usage since the previous snapshot and current RSS.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	cpu, rss := sampleProcess()

	var pct float64
	if m.initialized {
		dt := now.Sub(m.lastAt).Seconds()
		if dt > 0 {
			pct = float64(cpu-m.lastCPU) / float64(time.Second) / dt * 100
			if pct < 0 {
				pct = 0
			}
		}
	}

	m.lastCPU = cpu
	m.lastAt = now
	m.initialized = true

	return Snapshot{
		CPUPercent: pct,
		RSSBytes:   rss,
		RSS:        formatBytes(rss),
	}
}

func formatBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
