// Package proto implements the tiny TCP framing used between server and client.
//
// Handshake: client sends magic (4) + flags (1) + client send time (8 BE unix ns).
// Server replies with Hello (includes TimeInfo), client sends TypeTimeSync ack.
// After that both sides exchange frames: [1 byte type][4 byte BE length][payload].
//
// TypeAudio and TypeMic payloads are prefixed with an 8-byte big-endian
// send timestamp (sender local unix nanoseconds). Receivers correct latency
// using the clock offset from the handshake.
package proto

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Magic is sent by the client immediately after connecting.
const Magic = "ABR1"

// Client capability flags.
const (
	FlagWantsAudio byte = 1 << 0
	FlagSendsMic   byte = 1 << 1
)

// Frame types.
const (
	TypeHello    byte = 0 // server -> client, JSON stream parameters
	TypeAudio    byte = 1 // server -> client, PCM or encoded audio frame
	TypeMic      byte = 2 // client -> server, raw s16le PCM
	TypeReconfig byte = 3 // server -> client, JSON Hello with updated codec
	TypeTimeSync byte = 4 // client -> server, clock sync ack (see clocksync.go)
)

// MaxFrame is the largest accepted payload (1 s of 48 kHz stereo s16le is 192000 B).
const MaxFrame = 1 << 20

// Hello describes the PCM formats both directions use.
type Hello struct {
	Audio Format    `json:"audio"`
	Mic   Format    `json:"mic"`
	Codec CodecInfo `json:"codec"`
	Time  *TimeInfo `json:"time,omitempty"` // present on initial connect only
}

// CodecInfo describes compression on the server -> client audio stream.
type CodecInfo struct {
	Name        string `json:"name"`         // none | opus | flac
	BitrateKbps int    `json:"bitrate_kbps"` // opus target bitrate; 0 for none/flac
}

// Format is a raw PCM stream description (always s16le).
type Format struct {
	Rate     int `json:"rate"`
	Channels int `json:"channels"`
}

// WriteFrame writes one frame to w.
func WriteFrame(w io.Writer, t byte, payload []byte) error {
	hdr := make([]byte, 5)
	hdr[0] = t
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame reads one frame from r.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxFrame {
		return 0, nil, fmt.Errorf("proto: frame of %d bytes exceeds limit", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return hdr[0], buf, nil
}
