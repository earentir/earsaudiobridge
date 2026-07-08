// Package config loads the server and client YAML configuration files.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// WebUIConfig holds the local web dashboard bind address and poll intervals.
type WebUIConfig struct {
	Listen       string `yaml:"listen"`
	StatsPollMS  int    `yaml:"stats_poll_ms"`  // stats panel refresh
	PeakPollMS   int    `yaml:"peak_poll_ms"`   // peak meter refresh (server only)
	LayoutPollMS int    `yaml:"layout_poll_ms"` // devices / settings refresh
}

// ServerConfig is the full server side configuration.
type ServerConfig struct {
	Listen  string `yaml:"listen"`
	Backend string `yaml:"backend"` // auto | pipewire | pulse | native

	Web WebUIConfig `yaml:"web"`

	Audio struct {
		SinkName        string `yaml:"sink_name"`
		SinkDescription string `yaml:"sink_description"`
		SampleRate      int    `yaml:"sample_rate"`
		Channels        int    `yaml:"channels"`
	} `yaml:"audio"`

	Mic struct {
		Enabled           bool   `yaml:"enabled"`
		SinkName          string `yaml:"sink_name"`
		SourceName        string `yaml:"source_name"`
		SourceDescription string `yaml:"source_description"`
		SampleRate        int    `yaml:"sample_rate"`
		Channels          int    `yaml:"channels"`
	} `yaml:"mic"`

	// Native backend (Windows/macOS) device selection
	Native struct {
		CaptureDevice string `yaml:"capture_device"`
		MicOutDevice  string `yaml:"mic_out_device"`
	} `yaml:"native"`

	CleanupOnExit bool `yaml:"cleanup_on_exit"`

	Compression struct {
		Codec       string `yaml:"codec"`        // none | opus | flac
		BitrateKbps int    `yaml:"bitrate_kbps"` // opus level
	} `yaml:"compression"`

	// Outbound stream level and capture source (also adjustable in the web UI).
	Stream struct {
		GainPercent   int    `yaml:"gain_percent"`   // 0-150 software gain before encode
		Muted         bool   `yaml:"muted"`
		CaptureSource string `yaml:"capture_source"` // empty = default sink monitor
	} `yaml:"stream"`
}

func clampPercent(v, def int) int {
	if v < 0 {
		return 0
	}
	if v > 150 {
		return 150
	}
	if v == 0 {
		return def
	}
	return v
}

func clampPollMS(v, def int) int {
	if v <= 0 {
		return def
	}
	if v < 5 {
		return 5
	}
	if v > 60000 {
		return 60000
	}
	return v
}

func (w *WebUIConfig) applyDefaults(listen string, peak bool) {
	if w.Listen == "" {
		w.Listen = listen
	}
	w.StatsPollMS = clampPollMS(w.StatsPollMS, 250)
	if peak {
		w.PeakPollMS = clampPollMS(w.PeakPollMS, 100)
	}
	w.LayoutPollMS = clampPollMS(w.LayoutPollMS, 2000)
}

// ClientConfig is the full client side configuration.
type ClientConfig struct {
	Server  string `yaml:"server"`
	Backend string `yaml:"backend"` // auto | pulse | native

	Playback struct {
		Enabled     bool   `yaml:"enabled"`
		Device      string `yaml:"device"`
		LatencyMS   int    `yaml:"latency_ms"`
		GainPercent int    `yaml:"gain_percent"` // 0-150 software boost before output
		Muted       bool   `yaml:"muted"`
	} `yaml:"playback"`

	Mic struct {
		Enabled     bool   `yaml:"enabled"`
		Device      string `yaml:"device"`
		GainPercent int    `yaml:"gain_percent"` // 0-150 software boost before send
		Muted       bool   `yaml:"muted"`
	} `yaml:"mic"`

	Web WebUIConfig `yaml:"web"`

	ReconnectSeconds int `yaml:"reconnect_seconds"`
}

// LoadServer reads and validates a server config file, applying defaults.
func LoadServer(path string) (*ServerConfig, error) {
	c := &ServerConfig{}
	if err := load(path, c); err != nil {
		return nil, err
	}
	if c.Listen == "" {
		c.Listen = ":4600"
	}
	c.Web.applyDefaults(":4680", true)
	if c.Audio.SinkName == "" {
		c.Audio.SinkName = "earsaudiobridge"
	}
	if c.Audio.SinkName == "audiobridge" {
		c.Audio.SinkName = "earsaudiobridge"
	}
	if c.Audio.SinkDescription == "" {
		c.Audio.SinkDescription = "EarsAudioBridge_Output"
	}
	if c.Audio.SinkDescription == "AudioBridge_Output" {
		c.Audio.SinkDescription = "EarsAudioBridge_Output"
	}
	if c.Audio.SampleRate == 0 {
		c.Audio.SampleRate = 48000
	}
	if c.Audio.Channels == 0 {
		c.Audio.Channels = 2
	}
	if c.Mic.SinkName == "" {
		c.Mic.SinkName = "earsaudiobridge_mic"
	}
	if c.Mic.SinkName == "audiobridge_mic" {
		c.Mic.SinkName = "earsaudiobridge_mic"
	}
	if c.Mic.SourceName == "" {
		c.Mic.SourceName = "earsaudiobridge_mic_src"
	}
	if c.Mic.SourceName == "audiobridge_mic_src" {
		c.Mic.SourceName = "earsaudiobridge_mic_src"
	}
	if c.Mic.SourceDescription == "" {
		c.Mic.SourceDescription = "EarsAudioBridge_Microphone"
	}
	if c.Mic.SourceDescription == "AudioBridge_Microphone" {
		c.Mic.SourceDescription = "EarsAudioBridge_Microphone"
	}
	if c.Mic.SampleRate == 0 {
		c.Mic.SampleRate = 48000
	}
	if c.Mic.Channels == 0 {
		c.Mic.Channels = 1
	}
	if c.Compression.Codec == "" {
		c.Compression.Codec = "none"
	}
	if c.Compression.BitrateKbps == 0 {
		c.Compression.BitrateKbps = 96
	}
	c.Stream.GainPercent = clampPercent(c.Stream.GainPercent, 100)
	return c, nil
}

// LoadClient reads and validates a client config file, applying defaults.
func LoadClient(path string) (*ClientConfig, error) {
	c := &ClientConfig{}
	if err := load(path, c); err != nil {
		return nil, err
	}
	if c.Server == "" {
		return nil, fmt.Errorf("config: server address is required")
	}
	if c.Playback.LatencyMS == 0 {
		c.Playback.LatencyMS = 60
	}
	c.Playback.GainPercent = clampPercent(c.Playback.GainPercent, 100)
	c.Mic.GainPercent = clampPercent(c.Mic.GainPercent, 100)
	if c.ReconnectSeconds == 0 {
		c.ReconnectSeconds = 3
	}
	c.Web.applyDefaults("127.0.0.1:4680", true)
	return c, nil
}

func load(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}
