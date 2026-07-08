// Package pick selects the audio backend for server and client based on
// config and platform.
package pick

import (
	"fmt"
	"io"
	"runtime"

	"github.com/earentir/audiobridge/internal/audio"
	"github.com/earentir/audiobridge/internal/audio/native"
	"github.com/earentir/audiobridge/internal/audio/pipewire"
	"github.com/earentir/audiobridge/internal/audio/pulsebe"
	"github.com/earentir/audiobridge/internal/pa"
)

// ServerSystem returns the server backend for the given config value
// (auto, pipewire, pulse, native).
func ServerSystem(backend string) (audio.System, error) {
	switch backend {
	case "pipewire":
		return pipewire.New(), nil
	case "pulse":
		return pulsebe.New(), nil
	case "native":
		return native.NewSystem(), nil
	case "", "auto":
		if runtime.GOOS != "linux" {
			return native.NewSystem(), nil
		}
		if pipewire.Available() {
			return pipewire.New(), nil
		}
		if pulsebe.Available() {
			return pulsebe.New(), nil
		}
		return nil, fmt.Errorf("no audio backend found: need PipeWire (with pipewire-pulse) or PulseAudio")
	default:
		return nil, fmt.Errorf("unknown backend %q", backend)
	}
}

// clientBackend resolves auto for the client side.
func clientBackend(backend string) (string, error) {
	switch backend {
	case "pulse", "native":
		return backend, nil
	case "", "auto":
		if runtime.GOOS == "linux" && pulsebe.Available() {
			// Covers both PipeWire (via pipewire-pulse) and PulseAudio.
			return "pulse", nil
		}
		if native.Available() {
			return "native", nil
		}
		return "", fmt.Errorf("no client backend available: no Pulse server reachable and native backend not compiled in")
	default:
		return "", fmt.Errorf("unknown backend %q", backend)
	}
}

// ClientPlayer opens a playback stream on the resolved backend.
func ClientPlayer(backend, device string, f audio.Format, latencyMS int) (io.WriteCloser, func(), string, error) {
	b, err := clientBackend(backend)
	if err != nil {
		return nil, nil, "", err
	}
	var w io.WriteCloser
	var stop func()
	if b == "pulse" {
		w, stop, err = pulsebe.NewPlayer(device, f, latencyMS)
	} else {
		w, stop, err = native.NewPlayer(device, f, latencyMS)
	}
	return w, stop, b, err
}

// ClientRecorder opens a mic capture stream on the resolved backend.
func ClientRecorder(backend, device string, f audio.Format) (io.ReadCloser, func(), string, error) {
	b, err := clientBackend(backend)
	if err != nil {
		return nil, nil, "", err
	}
	var r io.ReadCloser
	var stop func()
	if b == "pulse" {
		r, stop, err = pulsebe.NewRecorder(device, f)
	} else {
		r, stop, err = native.NewRecorder(device, f, false)
	}
	return r, stop, b, err
}

// ClientSupportsVolume reports whether OS device volume control is available.
func ClientSupportsVolume(backend string) bool {
	b, err := clientBackend(backend)
	if err != nil {
		return false
	}
	return b == "pulse"
}

func pulseDeviceID(kind, name string) string {
	if name != "" {
		return name
	}
	if kind == "sink" {
		return "@DEFAULT_SINK@"
	}
	return "@DEFAULT_SOURCE@"
}

// ClientSetVolume sets PulseAudio/PipeWire volume for a client playback or capture device.
func ClientSetVolume(backend, kind, name string, percent int) error {
	b, err := clientBackend(backend)
	if err != nil {
		return err
	}
	if b != "pulse" {
		return fmt.Errorf("device volume is not supported on the %s backend", b)
	}
	paKind := "sink"
	if kind == "source" {
		paKind = "source"
	}
	return pa.SetVolume(paKind, pulseDeviceID(paKind, name), percent)
}

// ClientSetMute sets PulseAudio/PipeWire mute for a client device.
func ClientSetMute(backend, kind, name string, mute bool) error {
	b, err := clientBackend(backend)
	if err != nil {
		return err
	}
	if b != "pulse" {
		return fmt.Errorf("device mute is not supported on the %s backend", b)
	}
	paKind := "sink"
	if kind == "source" {
		paKind = "source"
	}
	return pa.SetMute(paKind, pulseDeviceID(paKind, name), mute)
}

// ClientDeviceVolume returns current OS volume for a client device.
func ClientDeviceVolume(backend, kind, name string) (volume int, muted bool, err error) {
	b, err := clientBackend(backend)
	if err != nil {
		return 0, false, err
	}
	if b != "pulse" {
		return 0, false, fmt.Errorf("device volume is not supported on the %s backend", b)
	}
	paKind := "sink"
	if kind == "source" {
		paKind = "source"
	}
	id := pulseDeviceID(paKind, name)
	volume, err = pa.GetVolume(paKind, id)
	if err != nil {
		return 0, false, err
	}
	muted, err = pa.GetMute(paKind, id)
	if err != nil {
		return volume, false, err
	}
	return volume, muted, nil
}

// ClientPlaybackDevices lists output devices for the resolved client backend.
func ClientPlaybackDevices(backend string) ([]audio.Device, error) {
	b, err := clientBackend(backend)
	if err != nil {
		return nil, err
	}
	if b == "pulse" {
		return pulsebe.PlaybackDevices()
	}
	return native.PlaybackDevices()
}

// ClientCaptureDevices lists input devices for the resolved client backend.
func ClientCaptureDevices(backend string) ([]audio.Device, error) {
	b, err := clientBackend(backend)
	if err != nil {
		return nil, err
	}
	if b == "pulse" {
		return pulsebe.CaptureDevices()
	}
	return native.CaptureDevices()
}
