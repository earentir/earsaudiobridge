package pulseclient

import (
	"fmt"
	"strings"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

// EnsureNullSink loads module-null-sink when missing. Returns module index, or 0 if already present.
func EnsureNullSink(name, description string) (uint32, error) {
	var moduleIndex uint32
	err := withClient(appName, func(c *pulse.Client) error {
		if _, err := c.SinkByID(name); err == nil {
			return nil
		}
		var reply proto.LoadModuleReply
		args := fmt.Sprintf("sink_name=%s sink_properties=device.description=%s",
			name, sanitizeDescription(description))
		if err := c.RawRequest(&proto.LoadModule{
			Name: "module-null-sink",
			Args: args,
		}, &reply); err != nil {
			return err
		}
		moduleIndex = reply.ModuleIndex
		return nil
	})
	return moduleIndex, err
}

// EnsureRemapSource exposes a sink monitor as a source via module-remap-source.
func EnsureRemapSource(masterSink, name, description string) (uint32, error) {
	var moduleIndex uint32
	err := withClient(appName, func(c *pulse.Client) error {
		if _, err := c.SourceByID(name); err == nil {
			return nil
		}
		var reply proto.LoadModuleReply
		args := fmt.Sprintf("master=%s.monitor source_name=%s source_properties=device.description=%s",
			masterSink, name, sanitizeDescription(description))
		if err := c.RawRequest(&proto.LoadModule{
			Name: "module-remap-source",
			Args: args,
		}, &reply); err != nil {
			return err
		}
		moduleIndex = reply.ModuleIndex
		return nil
	})
	return moduleIndex, err
}

// Unload removes a loaded module.
func Unload(moduleIndex uint32) error {
	if moduleIndex == 0 {
		return nil
	}
	return withClient(appName, func(c *pulse.Client) error {
		return c.RawRequest(&proto.UnloadModule{ModuleIndex: moduleIndex}, nil)
	})
}

// SetSinkDescription is a no-op when the sink already exists — the native protocol
// has no stable SetSinkProps in our client library.
func SetSinkDescription(name, description string) error {
	_ = name
	_ = description
	return nil
}

// SetSourceDescription is a no-op when the source already exists.
func SetSourceDescription(name, description string) error {
	_ = name
	_ = description
	return nil
}

func sanitizeDescription(s string) string {
	return strings.Join(strings.Fields(s), "_")
}
