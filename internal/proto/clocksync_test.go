package proto

import (
	"testing"
	"time"
)

func TestComputeClockSync(t *testing.T) {
	const offset = int64(100_000_000) // server 100ms ahead
	const rtt = int64(10_000_000)     // 10ms RTT
	t0 := int64(1_000_000_000_000)
	t1 := t0 + offset + rtt/2
	t2 := t1 + 1_000_000
	t3 := t0 + rtt

	gotOffset, gotRTT := ComputeClockSync(t0, t1, t2, t3)
	if gotOffset < offset-2_000_000 || gotOffset > offset+2_000_000 {
		t.Fatalf("offset: got %d want ~%d", gotOffset, offset)
	}
	if gotRTT < rtt-2_000_000 || gotRTT > rtt+2_000_000 {
		t.Fatalf("rtt: got %d want ~%d", gotRTT, rtt)
	}
}

func TestDownlinkUplinkLatency(t *testing.T) {
	offset := int64(100_000_000) // server 100ms ahead
	serverSend := time.Unix(0, 2_000_000_000)
	clientRecv := time.Unix(0, 1_905_000_000) // 5ms after client-equivalent of server send

	lat := DownlinkLatency(clientRecv, serverSend, offset)
	if lat < 4*time.Millisecond || lat > 6*time.Millisecond {
		t.Fatalf("downlink: %v", lat)
	}

	clientSend := time.Unix(0, 1_000_000_000)
	serverRecv := time.Unix(0, 1_105_000_000) // 5ms after server-equivalent of client send
	lat = UplinkLatency(serverRecv, clientSend, offset)
	if lat < 4*time.Millisecond || lat > 6*time.Millisecond {
		t.Fatalf("uplink: %v", lat)
	}
}
