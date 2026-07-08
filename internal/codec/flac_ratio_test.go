package codec

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/earentir/audiobridge/internal/audio"
)

func encodeAllBatches(c *Flac, pcm []byte, frames int) (wireTotal int, err error) {
	for i := 0; i < frames; i++ {
		w, err := c.Encode(pcm)
		if errors.Is(err, ErrBuffering) {
			continue
		}
		if err != nil {
			return 0, err
		}
		wireTotal += len(w)
	}
	return wireTotal, nil
}

func TestFlacCompressionRatio(t *testing.T) {
	f := audio.Format{Rate: 48000, Channels: 2}
	c, err := NewFlac(f)
	if err != nil {
		t.Fatal(err)
	}

	pcmBytes := 48000 / 50 * 2 * 2
	pcm := make([]byte, pcmBytes)
	for i := 0; i < pcmBytes/2; i++ {
		v := int16(8000 * math.Sin(float64(i)*0.07))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}

	const frames = 50 * flacBatchFrames
	wireTotal, err := encodeAllBatches(c, pcm, frames)
	if err != nil {
		t.Fatal(err)
	}

	ratio := float64(wireTotal) / float64(pcmBytes*frames)
	t.Logf("FLAC wire/pcm ratio: %.1f%%", ratio*100)
	if ratio >= 0.95 {
		t.Fatalf("expected meaningful FLAC compression, got ratio %.1f%%", ratio*100)
	}
}

func TestFlacCompressionDense(t *testing.T) {
	f := audio.Format{Rate: 48000, Channels: 2}
	c, err := NewFlac(f)
	if err != nil {
		t.Fatal(err)
	}
	pcmBytes := 48000 / 50 * 2 * 2
	pcm := make([]byte, pcmBytes)
	for i := 0; i < pcmBytes/2; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(i*7919)))
	}
	wire, err := flacEncodeBatch(c, pcm)
	if err != nil {
		t.Fatal(err)
	}
	ratio := float64(len(wire)) / float64(pcmBytes*flacBatchFrames)
	t.Logf("dense signal FLAC batch ratio: %.1f%%", ratio*100)
}
