package stats

import (
	"sync"
	"time"

	"github.com/earentir/audiobridge/internal/proto"
)

// NetSnapshot is one-way network timing (latency + jitter).
type NetSnapshot struct {
	LatencyMS     float64 `json:"latency_ms"`      // smoothed one-way delay
	LastLatencyMS float64 `json:"last_latency_ms"` // most recent sample
	JitterMS      float64 `json:"jitter_ms"`       // smoothed transit-time variation (RFC 3550 style)
	Samples       uint64  `json:"samples"`
}

// NetTiming tracks one-way network latency and jitter from stamped frames.
type NetTiming struct {
	mu sync.Mutex

	expected    time.Duration
	lastArrival time.Time
	lastTransit float64 // ms
	avgLatency  float64 // ms EMA
	lastLatency float64 // ms
	jitterEMA   float64 // ms
	samples     uint64
}

// NewNetTiming creates a tracker for frames arriving every expected interval.
func NewNetTiming(expectedFrame time.Duration) *NetTiming {
	return &NetTiming{expected: expectedFrame}
}

// Reset clears accumulated network timing.
func (n *NetTiming) Reset() {
	n.mu.Lock()
	n.lastArrival = time.Time{}
	n.lastTransit = 0
	n.avgLatency = 0
	n.lastLatency = 0
	n.jitterEMA = 0
	n.samples = 0
	n.mu.Unlock()
}

// RecordLatency records a one-way network delay (already offset-corrected).
func (n *NetTiming) RecordLatency(latency time.Duration, now time.Time) {
	if latency <= 0 {
		n.RecordArrival(now)
		return
	}
	transit := float64(latency.Microseconds()) / 1000

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.samples > 0 {
		d := transit - n.lastTransit
		if d < 0 {
			d = -d
		}
		n.jitterEMA += (d - n.jitterEMA) / 16
	}
	n.lastTransit = transit
	n.lastLatency = transit
	if n.samples == 0 {
		n.avgLatency = transit
	} else {
		n.avgLatency += (transit - n.avgLatency) / 16
	}

	n.recordArrivalLocked(now)
	n.samples++
}

// RecordTimed records a frame using sender local time and clock offset (server - client).
// fromServer is true for server->client (downlink).
func (n *NetTiming) RecordTimed(sendTime, now time.Time, offsetNS int64, fromServer bool) {
	var lat time.Duration
	if fromServer {
		lat = proto.DownlinkLatency(now, sendTime, offsetNS)
	} else {
		lat = proto.UplinkLatency(now, sendTime, offsetNS)
	}
	n.RecordLatency(lat, now)
}

// RecordArrival records inter-arrival jitter when no send timestamp is available.
func (n *NetTiming) RecordArrival(now time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.recordArrivalLocked(now)
	n.samples++
}

func (n *NetTiming) recordArrivalLocked(now time.Time) {
	if n.expected <= 0 || n.lastArrival.IsZero() {
		n.lastArrival = now
		return
	}
	interval := now.Sub(n.lastArrival)
	dev := float64(interval-n.expected) / float64(time.Millisecond)
	if dev < 0 {
		dev = -dev
	}
	if n.samples > 0 {
		n.jitterEMA += (dev - n.jitterEMA) / 16
	} else {
		n.jitterEMA = dev
	}
	n.lastArrival = now
}

// Snapshot returns the current network timing view.
func (n *NetTiming) Snapshot() NetSnapshot {
	n.mu.Lock()
	defer n.mu.Unlock()
	return NetSnapshot{
		LatencyMS:     n.avgLatency,
		LastLatencyMS: n.lastLatency,
		JitterMS:      n.jitterEMA,
		Samples:       n.samples,
	}
}
