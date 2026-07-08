package codec

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"

	"github.com/earentir/audiobridge/internal/audio"
)

const flacBatchFrames = 4 // 80 ms of 20 ms PCM input; larger FLAC blocks compress better

// Flac losslessly compresses batched PCM frames with FLAC.
type Flac struct {
	info      Info
	enc       *flac.Encoder
	buf       bytes.Buffer
	headerLen int
	rate      int
	channels  int
	samples   int // samples per channel per 20 ms input frame
	pcmBytes  int
	batchPCM  []byte
	batchSize int // pcm bytes per encoded FLAC frame
	chLayout  frame.Channels
}

// NewFlac creates a FLAC codec for the given PCM format.
func NewFlac(f audio.Format) (*Flac, error) {
	if f.Channels < 1 || f.Channels > 2 {
		return nil, fmt.Errorf("flac: unsupported channel count %d", f.Channels)
	}
	if f.Rate <= 0 {
		return nil, fmt.Errorf("flac: invalid sample rate %d", f.Rate)
	}

	samples := f.Rate / 50 // 20 ms frame
	chLayout := frame.ChannelsMono
	if f.Channels == 2 {
		chLayout = frame.ChannelsLR
	}
	pcmBytes := samples * f.Channels * 2
	batchSamples := samples * flacBatchFrames

	c := &Flac{
		info:      Info{Name: "flac"},
		rate:      f.Rate,
		channels:  f.Channels,
		samples:   samples,
		pcmBytes:  pcmBytes,
		batchSize: pcmBytes * flacBatchFrames,
		batchPCM:  make([]byte, 0, pcmBytes*flacBatchFrames),
		chLayout:  chLayout,
	}

	info := &meta.StreamInfo{
		BlockSizeMin:  uint16(batchSamples),
		BlockSizeMax:  uint16(batchSamples),
		SampleRate:    uint32(f.Rate),
		NChannels:     uint8(f.Channels),
		BitsPerSample: 16,
	}
	enc, err := flac.NewEncoder(&c.buf, info)
	if err != nil {
		return nil, err
	}
	c.enc = enc
	c.headerLen = c.buf.Len()
	return c, nil
}

// Info implements Codec.
func (c *Flac) Info() Info { return c.info }

// Encode implements Codec. Returns ErrBuffering until a full batch is ready.
func (c *Flac) Encode(pcm []byte) ([]byte, error) {
	if len(pcm) != c.pcmBytes {
		return nil, fmt.Errorf("flac: expected %d pcm bytes, got %d", c.pcmBytes, len(pcm))
	}

	c.batchPCM = append(c.batchPCM, pcm...)
	if len(c.batchPCM) < c.batchSize {
		return nil, ErrBuffering
	}

	batch := c.batchPCM[:c.batchSize]
	c.batchPCM = c.batchPCM[c.batchSize:]

	batchSamples := c.samples * flacBatchFrames
	start := c.buf.Len()
	f := &frame.Frame{
		Header: frame.Header{
			HasFixedBlockSize: true,
			BlockSize:         uint16(batchSamples),
			SampleRate:        uint32(c.rate),
			Channels:          c.chLayout,
			BitsPerSample:     16,
		},
		Subframes: make([]*frame.Subframe, c.channels),
	}
	for ch := 0; ch < c.channels; ch++ {
		samples := make([]int32, batchSamples)
		for i := range samples {
			off := (i*c.channels + ch) * 2
			samples[i] = int32(int16(binary.LittleEndian.Uint16(batch[off : off+2])))
		}
		f.Subframes[ch] = &frame.Subframe{
			SubHeader: frame.SubHeader{Pred: frame.PredVerbatim},
			Samples:   samples,
			NSamples:  batchSamples,
		}
	}
	if err := c.enc.WriteFrame(f); err != nil {
		return nil, err
	}
	wire := c.buf.Bytes()[start:]
	out := make([]byte, len(wire))
	copy(out, wire)
	c.buf.Truncate(c.headerLen)
	return out, nil
}

// Decode implements Codec.
func (c *Flac) Decode(wire []byte) ([]byte, error) {
	f, err := frame.Parse(bytes.NewReader(wire))
	if err != nil {
		return nil, err
	}
	f.Correlate()

	if f.Channels.Count() != c.channels {
		return nil, fmt.Errorf("flac: channel count mismatch")
	}
	nsamples := len(f.Subframes[0].Samples)
	outBytes := nsamples * c.channels * 2
	out := make([]byte, outBytes)
	for i := 0; i < nsamples; i++ {
		for ch := 0; ch < c.channels; ch++ {
			off := (i*c.channels + ch) * 2
			binary.LittleEndian.PutUint16(out[off:off+2], uint16(int16(f.Subframes[ch].Samples[i])))
		}
	}
	return out, nil
}
