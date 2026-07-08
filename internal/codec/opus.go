package codec

import (
	"encoding/binary"
	"fmt"

	"github.com/thesyncim/gopus"

	"github.com/earentir/audiobridge/internal/audio"
)

const (
	opusMinKbps = 16
	opusMaxKbps = 256
)

// Opus compresses 20 ms PCM frames with Opus.
type Opus struct {
	info      Info
	enc       *gopus.Encoder
	dec       *gopus.Decoder
	channels  int
	frameSize int // samples per channel per 20 ms frame
	encBuf    []byte
	pcmInt16  []int16
}

// NewOpus creates an Opus codec for the given format and target bitrate.
func NewOpus(f audio.Format, bitrateKbps int) (*Opus, error) {
	if bitrateKbps <= 0 {
		bitrateKbps = 96
	}
	if bitrateKbps < opusMinKbps {
		bitrateKbps = opusMinKbps
	}
	if bitrateKbps > opusMaxKbps {
		bitrateKbps = opusMaxKbps
	}
	if f.Rate != 48000 && f.Rate != 24000 && f.Rate != 16000 && f.Rate != 12000 && f.Rate != 8000 {
		return nil, fmt.Errorf("opus: unsupported sample rate %d", f.Rate)
	}
	if f.Channels < 1 || f.Channels > 2 {
		return nil, fmt.Errorf("opus: unsupported channel count %d", f.Channels)
	}

	enc, err := gopus.NewEncoder(gopus.EncoderConfig{
		SampleRate:  f.Rate,
		Channels:    f.Channels,
		Application: gopus.ApplicationAudio,
	})
	if err != nil {
		return nil, err
	}
	if err := enc.SetBitrate(bitrateKbps * 1000); err != nil {
		return nil, err
	}
	dec, err := gopus.NewDecoder(gopus.DecoderConfig{
		SampleRate: f.Rate,
		Channels:   f.Channels,
	})
	if err != nil {
		return nil, err
	}

	frameSize := f.Rate / 50 // 20 ms frame, per channel
	samples := frameSize * f.Channels
	return &Opus{
		info:      Info{Name: "opus", BitrateKbps: bitrateKbps},
		enc:       enc,
		dec:       dec,
		channels:  f.Channels,
		frameSize: frameSize,
		encBuf:    make([]byte, 4000),
		pcmInt16:  make([]int16, samples),
	}, nil
}

// Info implements Codec.
func (c *Opus) Info() Info { return c.info }

// Encode implements Codec.
func (c *Opus) Encode(pcm []byte) ([]byte, error) {
	want := c.frameSize * c.channels * 2
	if len(pcm) != want {
		return nil, fmt.Errorf("opus: expected %d pcm bytes, got %d", want, len(pcm))
	}
	for i := 0; i < c.frameSize*c.channels; i++ {
		c.pcmInt16[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	n, err := c.enc.EncodeInt16(c.pcmInt16, c.encBuf)
	if err != nil {
		return nil, err
	}
	out := make([]byte, n)
	copy(out, c.encBuf[:n])
	return out, nil
}

// Decode implements Codec.
func (c *Opus) Decode(wire []byte) ([]byte, error) {
	n, err := c.dec.DecodeInt16(wire, c.pcmInt16)
	if err != nil {
		return nil, err
	}
	count := n * c.channels
	out := make([]byte, count*2)
	for i := 0; i < count; i++ {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(c.pcmInt16[i]))
	}
	return out, nil
}

// OpusBitrateChoices returns supported bitrate presets in kbit/s.
func OpusBitrateChoices() []int {
	return []int{32, 48, 64, 96, 128, 160, 192, 256}
}
