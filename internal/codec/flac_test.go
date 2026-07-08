package codec

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/earentir/audiobridge/internal/audio"
)

func flacEncodeBatch(c *Flac, pcm []byte) ([]byte, error) {
	var batch []byte
	for i := 0; i < flacBatchFrames; i++ {
		batch = append(batch, pcm...)
	}
	for i := 0; i < flacBatchFrames; i++ {
		wire, err := c.Encode(pcm)
		if errors.Is(err, ErrBuffering) {
			continue
		}
		return wire, err
	}
	wire, err := c.Encode(pcm)
	return wire, err
}

func TestFlacRoundTrip(t *testing.T) {
	f := audio.Format{Rate: 48000, Channels: 2}
	c, err := NewFlac(f)
	if err != nil {
		t.Fatal(err)
	}

	pcm := make([]byte, 48000/50*2*2)
	for i := 0; i < len(pcm)/2; i++ {
		v := int16((i * 137) % 20000)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}

	wire, err := flacEncodeBatch(c, pcm)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) == 0 {
		t.Fatal("empty wire frame")
	}

	dec, err := NewFlac(f)
	if err != nil {
		t.Fatal(err)
	}
	back, err := dec.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	want := len(pcm) * flacBatchFrames
	if len(back) != want {
		t.Fatalf("length mismatch: got %d want %d", len(back), want)
	}
	batch := make([]byte, 0, want)
	for i := 0; i < flacBatchFrames; i++ {
		batch = append(batch, pcm...)
	}
	for i := range batch {
		if back[i] != batch[i] {
			t.Fatalf("sample mismatch at %d: got %d want %d", i, back[i], batch[i])
		}
	}
}

func TestFlacEncodeDoesNotGrowBuffer(t *testing.T) {
	f := audio.Format{Rate: 48000, Channels: 2}
	c, err := NewFlac(f)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, 48000/50*2*2)
	for i := 0; i < len(pcm)/2; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(i*3)))
	}
	for i := 0; i < 200; i++ {
		if _, err := c.Encode(pcm); err != nil && !errors.Is(err, ErrBuffering) {
			t.Fatal(err)
		}
	}
	if c.buf.Len() > c.headerLen+4096 {
		t.Fatalf("flac buffer grew to %d bytes (header %d)", c.buf.Len(), c.headerLen)
	}
}

func TestFlacMonoRoundTrip(t *testing.T) {
	f := audio.Format{Rate: 48000, Channels: 1}
	c, err := NewFlac(f)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, 48000/50*2)
	for i := 0; i < len(pcm)/2; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(i*50)))
	}
	wire, err := flacEncodeBatch(c, pcm)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := NewFlac(f)
	if err != nil {
		t.Fatal(err)
	}
	back, err := dec.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	batch := make([]byte, 0, len(pcm)*flacBatchFrames)
	for i := 0; i < flacBatchFrames; i++ {
		batch = append(batch, pcm...)
	}
	for i := range batch {
		if back[i] != batch[i] {
			t.Fatalf("sample mismatch at %d", i)
		}
	}
}
