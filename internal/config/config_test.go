package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerMigratesLegacyNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	err := os.WriteFile(path, []byte(`
audio:
  sink_name: audiobridge
  sink_description: AudioBridge_Output
mic:
  sink_name: audiobridge_mic
  source_name: audiobridge_mic_src
  source_description: AudioBridge_Microphone
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	c, err := LoadServer(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Audio.SinkName != "earsaudiobridge" {
		t.Fatalf("sink_name: got %q", c.Audio.SinkName)
	}
	if c.Audio.SinkDescription != "EarsAudioBridge_Output" {
		t.Fatalf("sink_description: got %q", c.Audio.SinkDescription)
	}
	if c.Mic.SourceName != "earsaudiobridge_mic_src" {
		t.Fatalf("source_name: got %q", c.Mic.SourceName)
	}
	if c.Mic.SourceDescription != "EarsAudioBridge_Microphone" {
		t.Fatalf("source_description: got %q", c.Mic.SourceDescription)
	}
}

func TestLoadServerWebAndStreamDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(path, []byte(`
stream:
  gain_percent: 80
  muted: true
  capture_source: foo.monitor
web:
  stats_poll_ms: 500
  peak_poll_ms: 50
  layout_poll_ms: 3000
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadServer(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Stream.GainPercent != 80 {
		t.Fatalf("gain: got %d", c.Stream.GainPercent)
	}
	if !c.Stream.Muted {
		t.Fatal("expected muted")
	}
	if c.Stream.CaptureSource != "foo.monitor" {
		t.Fatalf("capture: got %q", c.Stream.CaptureSource)
	}
	if c.Web.StatsPollMS != 500 || c.Web.PeakPollMS != 50 || c.Web.LayoutPollMS != 3000 {
		t.Fatalf("web polls: %+v", c.Web)
	}
}

func TestLoadClientWebDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(path, []byte(`
server: "127.0.0.1:4600"
web:
  stats_poll_ms: 150
  layout_poll_ms: 5000
`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Web.StatsPollMS != 150 || c.Web.LayoutPollMS != 5000 {
		t.Fatalf("web polls: %+v", c.Web)
	}
	if c.Web.Listen != "127.0.0.1:4680" {
		t.Fatalf("listen: got %q", c.Web.Listen)
	}
}
