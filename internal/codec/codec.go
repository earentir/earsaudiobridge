// Package codec encodes and decodes audio stream payloads for the wire.
package codec

import (
	"fmt"

	"github.com/earentir/audiobridge/internal/audio"
)

// Info describes the active stream codec.
type Info struct {
	Name        string `json:"name"`         // none | opus | flac
	BitrateKbps int    `json:"bitrate_kbps"` // opus target bitrate; 0 for none/flac
}

// Codec transforms PCM frames to and from wire payloads.
type Codec interface {
	Info() Info
	Encode(pcm []byte) ([]byte, error)
	Decode(wire []byte) ([]byte, error)
}

// New builds a codec for the given PCM format.
func New(info Info, f audio.Format) (Codec, error) {
	switch info.Name {
	case "", "none":
		return NewNone(), nil
	case "opus":
		return NewOpus(f, info.BitrateKbps)
	case "flac":
		return NewFlac(f)
	default:
		return nil, fmt.Errorf("unknown codec %q", info.Name)
	}
}

// DefaultInfo returns uncompressed PCM on the wire.
func DefaultInfo() Info { return Info{Name: "none"} }
