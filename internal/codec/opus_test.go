package codec

import (
	"encoding/binary"
	"testing"

	"github.com/earentir/audiobridge/internal/audio"
)

func TestOpusStereoRoundtrip(t *testing.T) {
	f := audio.Format{Rate: 48000, Channels: 2}
	c, err := NewOpus(f, 96)
	if err != nil {
		t.Fatal(err)
	}

	frameBytes := f.Rate * f.Channels * 2 / 50
	pcm := make([]byte, frameBytes)
	for i := 0; i < f.Rate/50; i++ {
		l := int16(12000 * (i%17) / 17)
		r := int16(-8000 * (i%13) / 13)
		binary.LittleEndian.PutUint16(pcm[(i*2)*2:], uint16(l))
		binary.LittleEndian.PutUint16(pcm[(i*2+1)*2:], uint16(r))
	}

	wire, err := c.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) == 0 {
		t.Fatal("empty opus packet")
	}

	out, err := c.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(pcm) {
		t.Fatalf("decode length %d, want %d", len(out), len(pcm))
	}

	// Stereo decode must preserve distinct channels (the old bug only wrote n mono samples).
	var lSum, rSum int64
	for i := 0; i < f.Rate/50; i++ {
		lSum += int64(int16(binary.LittleEndian.Uint16(out[(i*2)*2:])))
		rSum += int64(int16(binary.LittleEndian.Uint16(out[(i*2+1)*2:])))
	}
	if lSum*rSum >= 0 {
		t.Fatalf("expected opposite-sign channel energy, got L=%d R=%d", lSum, rSum)
	}
}
