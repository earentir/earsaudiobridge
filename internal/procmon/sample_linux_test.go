//go:build linux

package procmon

import (
	"testing"
	"time"
)

func TestMonitorSnapshot(t *testing.T) {
	m := New()
	s1 := m.Snapshot()
	if s1.RSSBytes == 0 {
		t.Fatal("expected non-zero RSS on linux")
	}
	time.Sleep(20 * time.Millisecond)
	_ = m.Snapshot()
}
