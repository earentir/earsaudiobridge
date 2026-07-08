//go:build !windows && !darwin

// Package native, stub for platforms where the native backend does not apply.
// Linux uses the pipewire/pulse backends instead.
package native

import (
	"fmt"
	"io"

	"github.com/earentir/audiobridge/internal/audio"
)

var errNoNative = fmt.Errorf("native backend is only available on Windows and macOS; use pipewire or pulse")

// Available reports whether the native backend is compiled in.
func Available() bool { return false }

// NewPlayer is unavailable on this platform.
func NewPlayer(string, audio.Format, int) (io.WriteCloser, func(), error) {
	return nil, nil, errNoNative
}

// NewRecorder is unavailable on this platform.
func NewRecorder(string, audio.Format, bool) (io.ReadCloser, func(), error) {
	return nil, nil, errNoNative
}

// System is unavailable on this platform.
type System struct{}

// NewSystem returns a native server backend stub.
func NewSystem() *System { return &System{} }

// Name implements audio.System.
func (s *System) Name() string { return "native" }

// Setup implements audio.System.
func (s *System) Setup(audio.SetupConfig) error { return errNoNative }

// DefaultCaptureTarget implements audio.System.
func (s *System) DefaultCaptureTarget() string { return "" }

// StartCapture implements audio.System.
func (s *System) StartCapture(string, audio.Format) (io.ReadCloser, func(), error) {
	return nil, nil, errNoNative
}

// StartMicOut implements audio.System.
func (s *System) StartMicOut(audio.Format) (io.WriteCloser, func(), error) {
	return nil, nil, errNoNative
}

// Sinks implements audio.System.
func (s *System) Sinks() ([]audio.Device, error) { return nil, errNoNative }

// Sources implements audio.System.
func (s *System) Sources() ([]audio.Device, error) { return nil, errNoNative }

// SupportsVolume implements audio.System.
func (s *System) SupportsVolume() bool { return false }

// SetVolume implements audio.System.
func (s *System) SetVolume(string, string, int) error { return errNoNative }

// SetMute implements audio.System.
func (s *System) SetMute(string, string, bool) error { return errNoNative }

// Cleanup implements audio.System.
func (s *System) Cleanup() error { return nil }

// PlaybackDevices is unavailable on this platform.
func PlaybackDevices() ([]audio.Device, error) { return nil, errNoNative }

// CaptureDevices is unavailable on this platform.
func CaptureDevices() ([]audio.Device, error) { return nil, errNoNative }
