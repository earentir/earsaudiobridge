package stats

import (
	"math"
	"sync"
	"time"
)

// Digital peak release: ~20 dB / 1.5 s (AES17-style), applied in linear amplitude.
const peakReleaseSeconds = 1.5

// peakHold implements broadcast-style peak metering: instant attack from audio
// frames, time-based release on read. Poll interval does not affect the peak
// captured from each audio frame.
type peakHold struct {
	mu      sync.Mutex
	latch   int // max sample peak from AddFrame since last read
	display int // ballistics output
	lastAt  time.Time
}

func (p *peakHold) add(samplePeak int) {
	if samplePeak <= 0 {
		return
	}
	p.mu.Lock()
	if samplePeak > p.latch {
		p.latch = samplePeak
	}
	p.mu.Unlock()
}

func (p *peakHold) reset() {
	p.mu.Lock()
	p.latch = 0
	p.display = 0
	p.lastAt = time.Time{}
	p.mu.Unlock()
}

// peek returns the current meter level without advancing ballistics.
func (p *peakHold) peek() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latch > p.display {
		return p.latch
	}
	return p.display
}

// read returns the peak meter level and advances release ballistics to now.
func (p *peakHold) read(now time.Time) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.latch > p.display {
		p.display = p.latch
	}
	p.latch = 0

	if !p.lastAt.IsZero() && p.display > 0 {
		dt := now.Sub(p.lastAt).Seconds()
		if dt > 0 {
			factor := math.Pow(10, -dt/peakReleaseSeconds)
			p.display = int(float64(p.display) * factor)
			if p.display < 1 {
				p.display = 0
			}
		}
	}
	p.lastAt = now
	return p.display
}
