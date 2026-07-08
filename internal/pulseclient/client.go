// Package pulseclient talks to PulseAudio or pipewire-pulse over the native protocol.
package pulseclient

import (
	"github.com/jfreymuth/pulse"
)

const appName = "EarsAudioBridge"

// Available reports whether a Pulse-compatible server accepts connections.
func Available() bool {
	c, err := openClient(appName)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func openClient(name string) (*pulse.Client, error) {
	return pulse.NewClient(pulse.ClientApplicationName(name))
}

func withClient(name string, fn func(*pulse.Client) error) error {
	c, err := openClient(name)
	if err != nil {
		return err
	}
	defer c.Close()
	return fn(c)
}
