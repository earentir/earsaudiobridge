package stats

import (
	"testing"
	"time"
)

func TestNetTimingLatencyAndJitter(t *testing.T) {
	n := NewNetTiming(20 * time.Millisecond)
	base := time.Now()

	n.RecordLatency(5*time.Millisecond, base)
	s := n.Snapshot()
	if s.LastLatencyMS < 4.9 || s.LastLatencyMS > 5.1 {
		t.Fatalf("latency: got %v", s.LastLatencyMS)
	}

	n.RecordLatency(6*time.Millisecond, base.Add(20*time.Millisecond))
	s = n.Snapshot()
	if s.Samples != 2 {
		t.Fatalf("samples: %d", s.Samples)
	}
}
