package proto

import (
	"testing"
	"time"
)

func TestWithSendTimeRoundTrip(t *testing.T) {
	now := time.Unix(0, 1_700_000_000_123_456_789)
	payload := []byte{1, 2, 3, 4}
	wrapped := WithSendTime(now, payload)

	got, data, ok := ParseSendTime(wrapped)
	if !ok {
		t.Fatal("expected ok")
	}
	if got.UnixNano() != now.UnixNano() {
		t.Fatalf("time: got %d want %d", got.UnixNano(), now.UnixNano())
	}
	if string(data) != string(payload) {
		t.Fatalf("payload: got %v want %v", data, payload)
	}
}

func TestParseSendTimeTooShort(t *testing.T) {
	_, _, ok := ParseSendTime([]byte{1, 2})
	if ok {
		t.Fatal("expected not ok")
	}
}
