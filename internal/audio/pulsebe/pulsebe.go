// Package pulsebe implements the PulseAudio backend using the native pulse protocol.
// It also works against PipeWire through pipewire-pulse.
package pulsebe

import (
	"fmt"
	"io"

	"github.com/earentir/audiobridge/internal/audio"
	"github.com/earentir/audiobridge/internal/pa"
	"github.com/earentir/audiobridge/internal/pulseclient"
)

// Available reports whether a Pulse-compatible server is reachable.
func Available() bool {
	return pulseclient.Available()
}

// System is the server-side PulseAudio backend.
type System struct {
	cfg     audio.SetupConfig
	modules []uint32
}

// New returns a PulseAudio server backend.
func New() *System { return &System{} }

// Name implements audio.System.
func (s *System) Name() string { return "pulse" }

// Setup implements audio.System.
func (s *System) Setup(cfg audio.SetupConfig) error {
	s.cfg = cfg
	id, err := pa.EnsureNullSink(cfg.SinkName, cfg.SinkDescription)
	if err != nil {
		return fmt.Errorf("output sink: %w", err)
	}
	s.track(id)

	if cfg.MicEnabled {
		id, err = pa.EnsureNullSink(cfg.MicSinkName, cfg.MicSinkName)
		if err != nil {
			return fmt.Errorf("mic sink: %w", err)
		}
		s.track(id)
		id, err = pa.EnsureRemapSource(cfg.MicSinkName, cfg.MicSourceName, cfg.MicSourceDesc)
		if err != nil {
			return fmt.Errorf("mic source: %w", err)
		}
		s.track(id)
	}
	return nil
}

func (s *System) track(moduleIndex uint32) {
	if moduleIndex != 0 {
		s.modules = append(s.modules, moduleIndex)
	}
}

// DefaultCaptureTarget implements audio.System.
func (s *System) DefaultCaptureTarget() string { return s.cfg.SinkName + ".monitor" }

// StartCapture implements audio.System.
func (s *System) StartCapture(target string, f audio.Format) (io.ReadCloser, func(), error) {
	device, err := resolveCaptureTarget(target, s.cfg.SinkName)
	if err != nil {
		return nil, nil, err
	}
	return pulseclient.NewRecorder(device, f, "EarsAudioBridge-Capture")
}

// StartMicOut implements audio.System.
func (s *System) StartMicOut(f audio.Format) (io.WriteCloser, func(), error) {
	return pulseclient.NewPlayer(s.cfg.MicSinkName, f, 20, "EarsAudioBridge-Mic")
}

// Sinks implements audio.System.
func (s *System) Sinks() ([]audio.Device, error) { return listDevices("sink") }

// Sources implements audio.System.
func (s *System) Sources() ([]audio.Device, error) { return listDevices("source") }

func listDevices(kind string) ([]audio.Device, error) {
	devs, err := pa.Devices(kind)
	if err != nil {
		return nil, err
	}
	out := make([]audio.Device, len(devs))
	for i, d := range devs {
		out[i] = audio.Device{Name: d.Name, Volume: d.Volume, Muted: d.Muted}
	}
	return out, nil
}

// SupportsVolume implements audio.System.
func (s *System) SupportsVolume() bool { return true }

// SetVolume implements audio.System.
func (s *System) SetVolume(kind, name string, percent int) error {
	return pa.SetVolume(kind, name, percent)
}

// SetMute implements audio.System.
func (s *System) SetMute(kind, name string, mute bool) error {
	return pa.SetMute(kind, name, mute)
}

// Cleanup implements audio.System.
func (s *System) Cleanup() error {
	var last error
	for _, id := range s.modules {
		if err := pa.Unload(id); err != nil {
			last = err
		}
	}
	return last
}

// NewPlayer returns a client-side playback writer.
func NewPlayer(device string, f audio.Format, latencyMS int) (io.WriteCloser, func(), error) {
	return pulseclient.NewPlayer(device, f, latencyMS, "EarsAudioBridge")
}

// NewRecorder returns a client-side mic capture reader.
func NewRecorder(device string, f audio.Format) (io.ReadCloser, func(), error) {
	return pulseclient.NewRecorder(device, f, "EarsAudioBridge-Mic")
}

// PlaybackDevices returns local output devices.
func PlaybackDevices() ([]audio.Device, error) {
	devs, err := pulseclient.PlaybackDevices()
	if err != nil {
		return nil, err
	}
	return mapDevices(devs), nil
}

// CaptureDevices returns local input devices.
func CaptureDevices() ([]audio.Device, error) {
	devs, err := pulseclient.CaptureDevices()
	if err != nil {
		return nil, err
	}
	return mapDevices(devs), nil
}

func mapDevices(devs []pulseclient.Device) []audio.Device {
	out := make([]audio.Device, len(devs))
	for i, d := range devs {
		desc := d.Name
		if desc == "" {
			desc = "System default"
		}
		out[i] = audio.Device{Name: d.Name, Description: desc, Volume: d.Volume, Muted: d.Muted}
	}
	return out
}

func resolveCaptureTarget(target, defaultSink string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("empty capture target")
	}
	if target == defaultSink {
		return defaultSink + ".monitor", nil
	}
	return target, nil
}
