// Package audio defines the backend-neutral types and the server-side
// audio system interface implemented by the pipewire, pulse and native backends.
package audio

import "io"

// Format is a raw PCM stream description (always s16le).
type Format struct {
	Rate     int
	Channels int
}

// Device is a sink or source with its current volume state.
type Device struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Volume      int    `json:"volume"`
	Muted       bool   `json:"muted"`
}

// SetupConfig describes the virtual devices a server backend should provide.
type SetupConfig struct {
	SinkName        string
	SinkDescription string

	MicEnabled    bool
	MicSinkName   string
	MicSourceName string
	MicSourceDesc string

	// Native backend only: physical/virtual devices to use instead of
	// creating them (Windows/macOS cannot create devices from userspace).
	NativeCaptureDevice string
	NativeMicOutDevice  string
}

// System is the server-side audio backend.
type System interface {
	Name() string

	// Setup creates or verifies the virtual devices.
	Setup(cfg SetupConfig) error

	// DefaultCaptureTarget is the target StartCapture should use initially.
	DefaultCaptureTarget() string

	// StartCapture starts capturing the given target and returns a PCM reader
	// plus a stop function.
	StartCapture(target string, f Format) (io.ReadCloser, func(), error)

	// StartMicOut returns a writer whose PCM ends up on the virtual mic source.
	StartMicOut(f Format) (io.WriteCloser, func(), error)

	// Sinks and Sources list devices for the web UI.
	Sinks() ([]Device, error)
	Sources() ([]Device, error)

	// SupportsVolume reports whether Set/GetVolume and mute work on this backend.
	SupportsVolume() bool
	SetVolume(kind, name string, percent int) error
	SetMute(kind, name string, mute bool) error

	// Cleanup removes devices created by Setup (best effort).
	Cleanup() error
}
