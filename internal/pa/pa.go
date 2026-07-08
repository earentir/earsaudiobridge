// Package pa provides PulseAudio device management via the native pulse protocol.
// It works against PulseAudio and against PipeWire through pipewire-pulse.
package pa

import (
	"github.com/earentir/audiobridge/internal/pulseclient"
)

// Device is a sink or source with its current volume state.
type Device struct {
	Name   string `json:"name"`
	Volume int    `json:"volume"`
	Muted  bool   `json:"muted"`
}

// ListNames returns the names of all objects of the given kind ("sinks", "sources", "modules").
func ListNames(kind string) ([]string, error) {
	return pulseclient.ListNames(kind)
}

// Devices returns all sinks or sources with volume and mute state.
// kind is "sink" or "source".
func Devices(kind string) ([]Device, error) {
	devs, err := pulseclient.Devices(kind)
	if err != nil {
		return nil, err
	}
	out := make([]Device, len(devs))
	for i, d := range devs {
		out[i] = Device{Name: d.Name, Volume: d.Volume, Muted: d.Muted}
	}
	return out, nil
}

// GetVolume returns the volume of a sink or source as a percentage.
func GetVolume(kind, name string) (int, error) {
	return pulseclient.GetVolume(kind, name)
}

// SetVolume sets the volume of a sink or source to a percentage.
func SetVolume(kind, name string, percent int) error {
	return pulseclient.SetVolume(kind, name, percent)
}

// GetMute returns the mute state of a sink or source.
func GetMute(kind, name string) (bool, error) {
	return pulseclient.GetMute(kind, name)
}

// SetMute sets the mute state of a sink or source.
func SetMute(kind, name string, mute bool) error {
	return pulseclient.SetMute(kind, name, mute)
}

// SetSinkDescription updates the human-readable name of an existing sink.
func SetSinkDescription(name, description string) error {
	return pulseclient.SetSinkDescription(name, description)
}

// SetSourceDescription updates the human-readable name of an existing source.
func SetSourceDescription(name, description string) error {
	return pulseclient.SetSourceDescription(name, description)
}

// EnsureNullSink loads a module-null-sink if a sink with that name does not exist yet.
// It returns the loaded module index, or 0 if the sink already existed.
func EnsureNullSink(name, description string) (uint32, error) {
	return pulseclient.EnsureNullSink(name, description)
}

// EnsureRemapSource exposes the monitor of a null sink as a regular input device.
func EnsureRemapSource(masterSink, name, description string) (uint32, error) {
	return pulseclient.EnsureRemapSource(masterSink, name, description)
}

// Unload unloads a module by index.
func Unload(moduleIndex uint32) error {
	return pulseclient.Unload(moduleIndex)
}
