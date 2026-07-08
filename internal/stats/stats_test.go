package stats

import (
	"testing"
	"time"
)

func pcmWithPeak(peak int16) []byte {
	out := make([]byte, 960*2*2) // 20ms stereo 48k
	for i := 0; i < len(out); i += 2 {
		out[i] = byte(peak)
		out[i+1] = byte(peak >> 8)
	}
	return out
}

func TestTakePeakFastPollMatchesFramePeak(t *testing.T) {
	s := &Stream{}
	s.AddFrame(pcmWithPeak(28000), len(pcmWithPeak(28000)))

	gotFast := s.TakePeak()
	if gotFast < 27000 {
		t.Fatalf("fast poll peak = %d, want ~28000", gotFast)
	}

	s.AddFrame(pcmWithPeak(30000), len(pcmWithPeak(30000)))
	time.Sleep(2 * time.Millisecond)
	got := s.TakePeak()
	if got < 29000 {
		t.Fatalf("peak after loud frame = %d, want ~30000", got)
	}
}

func TestTakePeakSlowPollAccumulatesFrames(t *testing.T) {
	s := &Stream{}
	for range 5 {
		s.AddFrame(pcmWithPeak(25000), len(pcmWithPeak(25000)))
	}
	got := s.TakePeak()
	if got < 24000 {
		t.Fatalf("slow poll peak = %d, want ~25000", got)
	}
}

func TestTakePeakEmptyPollDoesNotFakeZero(t *testing.T) {
	s := &Stream{}
	s.AddFrame(pcmWithPeak(32000), len(pcmWithPeak(32000)))

	first := s.TakePeak()
	if first < 31000 {
		t.Fatalf("first peak = %d", first)
	}

	// Fast polls with no new audio should decay slowly, not snap to zero.
	for range 3 {
		v := s.TakePeak()
		if v == 0 {
			t.Fatal("peak snapped to zero between audio frames")
		}
	}
}

func TestPeakHoldRelease(t *testing.T) {
	var p peakHold
	now := time.Now()
	p.add(30000)
	if v := p.read(now); v < 29000 {
		t.Fatalf("attack: got %d", v)
	}

	// After release time, meter should fall noticeably.
	later := now.Add(time.Duration(peakReleaseSeconds * float64(time.Second)))
	v := p.read(later)
	if v > 4000 {
		t.Fatalf("after %.1fs release expected << 30000, got %d", peakReleaseSeconds, v)
	}
}
