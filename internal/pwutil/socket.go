// Package pwutil detects a running PipeWire core without shelling out.
package pwutil

import (
	"net"
	"os"
	"path/filepath"
)

// SocketPath returns the PipeWire core socket for the current user session.
func SocketPath() string {
	if p := os.Getenv("PIPEWIRE_RUNTIME_DIR"); p != "" {
		return filepath.Join(p, "pipewire-0")
	}
	if r := os.Getenv("XDG_RUNTIME_DIR"); r != "" {
		return filepath.Join(r, "pipewire-0")
	}
	return ""
}

// Available reports whether the PipeWire core socket exists and accepts connections.
func Available() bool {
	path := SocketPath()
	if path == "" {
		return false
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
