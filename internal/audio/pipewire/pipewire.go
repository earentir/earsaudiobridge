// Package pipewire implements the PipeWire server backend. PipeWire is detected
// via its unix socket; audio I/O and device control use the native Pulse protocol
// through pipewire-pulse (standard on desktop PipeWire installs).
package pipewire

import (
	"fmt"
	"io"

	"github.com/earentir/audiobridge/internal/audio"
	"github.com/earentir/audiobridge/internal/pa"
	"github.com/earentir/audiobridge/internal/pulseclient"
	"github.com/earentir/audiobridge/internal/pwutil"
)

// Available reports whether PipeWire is running and pipewire-pulse is reachable.
func Available() bool {
	return pwutil.Available() && pulseclient.Available()
}

// System is the server-side PipeWire backend.
type System struct {
	cfg     audio.SetupConfig
	modules []uint32
}

// New returns a PipeWire server backend.
func New() *System { return &System{} }

// Name implements audio.System.
func (s *System) Name() string { return "pipewire" }

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

func resolveCaptureTarget(target, defaultSink string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("empty capture target")
	}
	if target == defaultSink {
		return defaultSink + ".monitor", nil
	}
	return target, nil
}
