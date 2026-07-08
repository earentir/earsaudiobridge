package stats

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"
)

// Snapshot is a point-in-time view of one audio stream.
type Snapshot struct {
	FramesPerSec         float64 `json:"frames_per_sec"`
	BandwidthKbps        float64 `json:"bandwidth_kbps"`
	BitrateKbps          float64 `json:"bitrate_kbps"` // alias for bandwidth_kbps
	PcmBandwidthKbps     float64 `json:"pcm_bandwidth_kbps"`
	SessionBandwidthKbps float64 `json:"session_bandwidth_kbps"`
	Peak                 int     `json:"peak"` // 0-32767
	RMS                  int     `json:"rms"`  // 0-32767
	Frames               uint64  `json:"frames"`
	Bytes                uint64  `json:"bytes"`
	WireBytes            uint64  `json:"wire_bytes"`
	Dropped              uint64  `json:"dropped"`
	TotalWire            string  `json:"total_wire"`
	TotalPCM             string  `json:"total_pcm"`
	CompressionRatio     float64 `json:"compression_ratio"`      // wire/pcm bandwidth (1 = none)
	CompressionSaving    float64 `json:"compression_saving_pct"` // (1 - ratio) * 100
	CumulativeRatio      float64 `json:"cumulative_ratio"`       // total wire / total pcm
}

// Stream accumulates frame counters and audio levels.
type Stream struct {
	mu sync.Mutex

	frames    uint64
	pcmBytes  uint64
	wireBytes uint64
	dropped   uint64
	peakMeter peakHold
	rms       int

	startedAt     time.Time
	lastAt        time.Time
	lastFrames    uint64
	lastWireBytes uint64
	lastPcmBytes  uint64

	cachedRates Snapshot
	hasCached   bool
}

const minRateInterval = 100 * time.Millisecond

// Reset clears counters and rate baselines (e.g. after a codec change).
func (s *Stream) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames = 0
	s.pcmBytes = 0
	s.wireBytes = 0
	s.dropped = 0
	s.peakMeter.reset()
	s.rms = 0
	s.startedAt = time.Time{}
	s.lastAt = time.Time{}
	s.lastFrames = 0
	s.lastWireBytes = 0
	s.lastPcmBytes = 0
	s.hasCached = false
}

// AddFrame records one PCM payload sent or received on the wire.
// wireLen is the encoded frame size; pass len(pcm) when uncompressed.
func (s *Stream) AddFrame(pcm []byte, wireLen int) {
	peak, rms := pcmLevels(pcm)
	if wireLen <= 0 {
		wireLen = len(pcm)
	}
	s.mu.Lock()
	if s.frames == 0 {
		s.startedAt = time.Now()
	}
	s.frames++
	s.pcmBytes += uint64(len(pcm))
	s.wireBytes += uint64(wireLen)
	s.peakMeter.add(peak)
	s.rms = rms
	s.mu.Unlock()
}

// AddDropped records frames that were not delivered.
func (s *Stream) AddDropped(n uint64) {
	s.mu.Lock()
	s.dropped += n
	s.mu.Unlock()
}

// Snapshot returns bandwidth rates and totals since the previous rate snapshot.
// Peak levels are not consumed; use TakePeak for meters.
func (s *Stream) Snapshot() Snapshot {
	return s.rateSnapshot()
}

// RateSnapshot is an alias for Snapshot.
func (s *Stream) RateSnapshot() Snapshot {
	return s.rateSnapshot()
}

// TakePeak returns the current peak meter level (instant attack, timed release).
// Safe at any poll interval — audio frames latch peaks between reads.
func (s *Stream) TakePeak() int {
	return s.peakMeter.read(time.Now())
}

func (s *Stream) rateSnapshot() Snapshot {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastAt.IsZero() {
		s.lastAt = now
		s.lastFrames = s.frames
		s.lastWireBytes = s.wireBytes
		s.lastPcmBytes = s.pcmBytes
		return s.makeSnapshot(0, 0, 0)
	}

	elapsed := now.Sub(s.lastAt).Seconds()
	if elapsed > 0 && elapsed < minRateInterval.Seconds() && s.hasCached {
		return s.withFreshTotals(s.cachedRates)
	}

	var fps, wireKbps, pcmKbps float64
	if elapsed > 0 {
		fps = float64(s.frames-s.lastFrames) / elapsed
		wireKbps = float64(s.wireBytes-s.lastWireBytes) * 8 / elapsed / 1000
		pcmKbps = float64(s.pcmBytes-s.lastPcmBytes) * 8 / elapsed / 1000
		s.lastAt = now
		s.lastFrames = s.frames
		s.lastWireBytes = s.wireBytes
		s.lastPcmBytes = s.pcmBytes
	}

	out := s.makeSnapshot(fps, wireKbps, pcmKbps)
	if elapsed > 0 {
		s.cachedRates = out
		s.hasCached = true
	}
	return out
}

func (s *Stream) withFreshTotals(base Snapshot) Snapshot {
	out := base
	out.Peak = s.peakMeter.peek()
	out.RMS = s.rms
	out.Frames = s.frames
	out.Bytes = s.wireBytes
	out.WireBytes = s.wireBytes
	out.Dropped = s.dropped
	out.TotalWire = formatBytes(s.wireBytes)
	out.TotalPCM = formatBytes(s.pcmBytes)
	out.SessionBandwidthKbps = s.sessionBandwidthKbps()
	if s.pcmBytes > 0 {
		out.CumulativeRatio = float64(s.wireBytes) / float64(s.pcmBytes)
	}
	return out
}

func (s *Stream) sessionBandwidthKbps() float64 {
	if s.startedAt.IsZero() || s.wireBytes == 0 {
		return 0
	}
	sec := time.Since(s.startedAt).Seconds()
	if sec <= 0 {
		return 0
	}
	return float64(s.wireBytes) * 8 / sec / 1000
}

func (s *Stream) makeSnapshot(fps, wireKbps, pcmKbps float64) Snapshot {
	var ratio, saving, cumulative float64
	if pcmKbps > 0 {
		ratio = wireKbps / pcmKbps
		saving = (1 - ratio) * 100
	}
	if s.pcmBytes > 0 {
		cumulative = float64(s.wireBytes) / float64(s.pcmBytes)
	}
	return Snapshot{
		FramesPerSec:         fps,
		BandwidthKbps:        wireKbps,
		BitrateKbps:          wireKbps,
		PcmBandwidthKbps:     pcmKbps,
		SessionBandwidthKbps: s.sessionBandwidthKbps(),
		Peak:                 s.peakMeter.peek(),
		RMS:                  s.rms,
		Frames:               s.frames,
		Bytes:                s.wireBytes,
		WireBytes:            s.wireBytes,
		Dropped:              s.dropped,
		TotalWire:            formatBytes(s.wireBytes),
		TotalPCM:             formatBytes(s.pcmBytes),
		CompressionRatio:     ratio,
		CompressionSaving:    saving,
		CumulativeRatio:      cumulative,
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

func pcmLevels(payload []byte) (peak, rms int) {
	if len(payload) < 2 {
		return 0, 0
	}
	var sum int64
	n := 0
	for i := 0; i+1 < len(payload); i += 2 {
		v := int32(int16(binary.LittleEndian.Uint16(payload[i : i+2])))
		av := int(v)
		if av < 0 {
			av = -av
		}
		if av > peak {
			peak = av
		}
		sum += int64(v) * int64(v)
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return peak, int(math.Sqrt(float64(sum) / float64(n)))
}
