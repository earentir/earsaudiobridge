package audio

import (
	"encoding/binary"
	"testing"
)

func TestApplyGain(t *testing.T) {
	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:2], uint16(int16(20000)))
	neg := int16(-20000)
	binary.LittleEndian.PutUint16(pcm[2:4], uint16(neg))

	ApplyGain(pcm, 0.5)

	if got := int16(binary.LittleEndian.Uint16(pcm[0:2])); got != 10000 {
		t.Fatalf("positive sample: got %d want 10000", got)
	}
	if got := int16(binary.LittleEndian.Uint16(pcm[2:4])); got != -10000 {
		t.Fatalf("negative sample: got %d want -10000", got)
	}
}
