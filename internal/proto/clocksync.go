package proto

import (
	"encoding/binary"
	"time"
)

// HandshakeLen is magic (4) + flags (1) + client send time (8).
const HandshakeLen = 13

// TimeInfo is included in Hello so the client can compute clock offset and RTT.
type TimeInfo struct {
	ClientSendNS int64 `json:"client_send_ns"`
	ServerRecvNS int64 `json:"server_recv_ns"`
	ServerSendNS int64 `json:"server_send_ns"`
}

// TimeAck is sent client -> server after Hello is received.
type TimeAck struct {
	ClientRecvNS int64 `json:"client_recv_ns"`
}

// ClockView is the synchronized clock relationship exposed to the web UI.
type ClockView struct {
	OffsetMS     float64 `json:"offset_ms"`      // server minus client (positive = server ahead)
	RTTMS        float64 `json:"rtt_ms"`         // handshake round-trip estimate
	ServerTimeNS int64   `json:"server_time_ns"` // server wall at sync (unix nano)
	ClientTimeNS int64   `json:"client_time_ns"` // client wall at sync (unix nano)
	Synced       bool    `json:"synced"`
}

// BuildHandshake returns the client connect preamble with a send timestamp.
func BuildHandshake(flags byte, clientSend time.Time) []byte {
	out := make([]byte, HandshakeLen)
	copy(out, Magic)
	out[4] = flags
	binary.BigEndian.PutUint64(out[5:], uint64(clientSend.UnixNano()))
	return out
}

// ParseHandshake reads flags and client send time from the connect preamble.
func ParseHandshake(hs []byte) (flags byte, clientSendNS int64, ok bool) {
	if len(hs) < HandshakeLen || string(hs[:4]) != Magic {
		return 0, 0, false
	}
	return hs[4], int64(binary.BigEndian.Uint64(hs[5:])), true
}

// ComputeClockSync estimates clock offset and RTT from a four-point exchange.
// t0 client send, t1 server recv, t2 server send, t3 client recv (all unix nanos).
// offset = server - client; RTT = (t3-t0) - (t2-t1).
func ComputeClockSync(t0, t1, t2, t3 int64) (offsetNS, rttNS int64) {
	offsetNS = ((t1 - t0) + (t2 - t3)) / 2
	rttNS = (t3 - t0) - (t2 - t1)
	if rttNS < 0 {
		rttNS = 0
	}
	return offsetNS, rttNS
}

// ClockViewFromSync builds a UI snapshot from sync timestamps.
func ClockViewFromSync(t0, t1, t2, t3, offsetNS, rttNS int64) ClockView {
	return ClockView{
		OffsetMS:     float64(offsetNS) / 1e6,
		RTTMS:        float64(rttNS) / 1e6,
		ServerTimeNS: t2,
		ClientTimeNS: t0,
		Synced:       true,
	}
}

// DownlinkLatency returns server->client one-way delay using synced clocks.
// offsetNS is server minus client.
func DownlinkLatency(recv, send time.Time, offsetNS int64) time.Duration {
	if recv.IsZero() || send.IsZero() {
		return 0
	}
	latNS := recv.UnixNano() - send.UnixNano() + offsetNS
	if latNS < 0 {
		return 0
	}
	return time.Duration(latNS)
}

// UplinkLatency returns client->server one-way delay using synced clocks.
func UplinkLatency(recv, send time.Time, offsetNS int64) time.Duration {
	if recv.IsZero() || send.IsZero() {
		return 0
	}
	latNS := recv.UnixNano() - send.UnixNano() - offsetNS
	if latNS < 0 {
		return 0
	}
	return time.Duration(latNS)
}

// EstimateClientNow estimates the client's current wall time from server now.
func EstimateClientNow(serverNow time.Time, offsetNS int64) time.Time {
	return time.Unix(0, serverNow.UnixNano()-offsetNS)
}

// EstimateServerNow estimates the server's current wall time from client now.
func EstimateServerNow(clientNow time.Time, offsetNS int64) time.Time {
	return time.Unix(0, clientNow.UnixNano()+offsetNS)
}
