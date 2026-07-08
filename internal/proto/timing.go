package proto

import (
	"encoding/binary"
	"time"
)

// TimeHeaderLen is the send-time prefix on timed audio/mic payloads (BE unix nanos).
const TimeHeaderLen = 8

// WithSendTime prefixes payload with the send timestamp (wall clock, unix nanoseconds).
func WithSendTime(now time.Time, payload []byte) []byte {
	out := make([]byte, TimeHeaderLen+len(payload))
	binary.BigEndian.PutUint64(out, uint64(now.UnixNano()))
	copy(out[TimeHeaderLen:], payload)
	return out
}

// ParseSendTime splits a timed payload. ok is false when the buffer is too short.
func ParseSendTime(payload []byte) (sendTime time.Time, data []byte, ok bool) {
	if len(payload) < TimeHeaderLen {
		return time.Time{}, payload, false
	}
	ns := int64(binary.BigEndian.Uint64(payload[:TimeHeaderLen]))
	return time.Unix(0, ns), payload[TimeHeaderLen:], true
}
