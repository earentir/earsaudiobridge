package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/earentir/audiobridge/internal/audio"
	"github.com/earentir/audiobridge/internal/codec"
	"github.com/earentir/audiobridge/internal/pa"
	"github.com/earentir/audiobridge/internal/procmon"
)

//go:embed web
var webFS embed.FS

type webPollView struct {
	StatsPollMS  int `json:"stats_poll_ms"`
	PeakPollMS   int `json:"peak_poll_ms"`
	LayoutPollMS int `json:"layout_poll_ms"`
}

type stateResponse struct {
	Version         string           `json:"version"`
	Backend         string           `json:"backend"`
	CaptureTargets  []string         `json:"capture_targets"`
	CaptureSource   string           `json:"capture_source"`
	OutputSink      string           `json:"output_sink"`
	MicSource       string           `json:"mic_source"`
	MicEnabled      bool             `json:"mic_enabled"`
	VolumeSupported bool             `json:"volume_supported"`
	Clients         int              `json:"clients"`
	Process         procmon.Snapshot `json:"process"`
	Compression     compressionView  `json:"compression"`
	AudioFormat     string           `json:"audio_format"`
	MicFormat       string           `json:"mic_format"`
	StreamGain      int              `json:"stream_gain"`
	StreamMuted     bool             `json:"stream_muted"`
	Sinks           []audio.Device   `json:"sinks"`
	Sources         []audio.Device   `json:"sources"`
	Web             webPollView      `json:"web"`
}

type compressionView struct {
	Codec          string `json:"codec"`
	BitrateKbps    int    `json:"bitrate_kbps"`
	BitrateChoices []int  `json:"bitrate_choices"`
}

// ServeWeb starts the embedded web UI on the configured port.
func (s *Server) ServeWeb() error {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("GET /api/state", s.apiState)
	mux.HandleFunc("GET /api/stats", s.apiStats)
	mux.HandleFunc("GET /api/peaks", s.apiPeaks)
	mux.HandleFunc("POST /api/volume", s.apiVolume)
	mux.HandleFunc("POST /api/mute", s.apiMute)
	mux.HandleFunc("POST /api/stream-gain", s.apiStreamGain)
	mux.HandleFunc("POST /api/capture", s.apiCapture)
	mux.HandleFunc("POST /api/compression", s.apiCompression)

	log.Printf("web ui on %s", s.cfg.Web.Listen)
	return http.ListenAndServe(s.cfg.Web.Listen, mux)
}

type statsResponse struct {
	Clients     int             `json:"clients"`
	MicEnabled  bool            `json:"mic_enabled"`
	AudioFormat string          `json:"audio_format"`
	MicFormat   string          `json:"mic_format"`
	Compression compressionView `json:"compression"`
	Stats       StreamStats     `json:"stats"`
}

func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	comp := s.Compression()
	writeJSON(w, statsResponse{
		Clients:     s.ClientCount(),
		MicEnabled:  s.cfg.Mic.Enabled,
		AudioFormat: formatLabel(s.cfg.Audio.SampleRate, s.cfg.Audio.Channels),
		MicFormat:   formatLabel(s.cfg.Mic.SampleRate, s.cfg.Mic.Channels),
		Compression: compressionView{
			Codec:          comp.Name,
			BitrateKbps:    comp.BitrateKbps,
			BitrateChoices: codec.OpusBitrateChoices(),
		},
		Stats: s.Stats(),
	})
}

func (s *Server) apiPeaks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"peaks": s.Peaks(),
	})
}

func (s *Server) apiState(w http.ResponseWriter, r *http.Request) {
	sinks, err := s.sys.Sinks()
	if err != nil {
		httpErr(w, err)
		return
	}
	sources, err := s.sys.Sources()
	if err != nil {
		httpErr(w, err)
		return
	}
	comp := s.Compression()
	writeJSON(w, stateResponse{
		Version:         s.version,
		Backend:         s.sys.Name(),
		CaptureTargets:  s.captureTargets(sinks, sources),
		CaptureSource:   s.CaptureSource(),
		OutputSink:      s.cfg.Audio.SinkName,
		MicSource:       s.cfg.Mic.SourceName,
		MicEnabled:      s.cfg.Mic.Enabled,
		VolumeSupported: s.sys.SupportsVolume(),
		Clients:         s.ClientCount(),
		Process:         s.proc.Snapshot(),
		Compression: compressionView{
			Codec:          comp.Name,
			BitrateKbps:    comp.BitrateKbps,
			BitrateChoices: codec.OpusBitrateChoices(),
		},
		StreamGain:  s.StreamGainPercent(),
		StreamMuted: s.StreamMuted(),
		AudioFormat: formatLabel(s.cfg.Audio.SampleRate, s.cfg.Audio.Channels),
		MicFormat:   formatLabel(s.cfg.Mic.SampleRate, s.cfg.Mic.Channels),
		Sinks:       sinks,
		Sources:     sources,
		Web: webPollView{
			StatsPollMS:  s.cfg.Web.StatsPollMS,
			PeakPollMS:   s.cfg.Web.PeakPollMS,
			LayoutPollMS: s.cfg.Web.LayoutPollMS,
		},
	})
}

func formatLabel(rate, channels int) string {
	ch := "mono"
	if channels == 2 {
		ch = "stereo"
	} else if channels > 2 {
		ch = fmt.Sprintf("%d ch", channels)
	}
	return fmt.Sprintf("%d Hz %s s16le", rate, ch)
}

// captureTargets lists valid StartCapture targets for the web UI dropdown.
// Both Linux backends expose Pulse-style source names (including .monitor endpoints).
func (s *Server) captureTargets(sinks, sources []audio.Device) []string {
	if s.sys.Name() == "pipewire" || s.sys.Name() == "pulse" {
		if names, err := pa.ListNames("sources"); err == nil && len(names) > 0 {
			return ensureCaptureTarget(names, s.CaptureSource())
		}
	}
	var t []string
	for _, d := range sources {
		t = append(t, d.Name)
	}
	if s.sys.Name() != "pulse" {
		for _, d := range sinks {
			t = append(t, d.Name)
			t = append(t, d.Name+".monitor")
		}
	}
	return ensureCaptureTarget(t, s.CaptureSource())
}

func ensureCaptureTarget(targets []string, current string) []string {
	if current == "" {
		return targets
	}
	for _, name := range targets {
		if name == current {
			return targets
		}
	}
	out := make([]string, 0, len(targets)+1)
	out = append(out, current)
	out = append(out, targets...)
	return out
}

func (s *Server) apiVolume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Percent int    `json:"percent"`
	}
	if !decode(w, r, &req) || !validKind(w, req.Kind) {
		return
	}
	if err := s.SetDeviceVolume(req.Kind, req.Name, req.Percent); err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiStreamGain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Percent *int  `json:"percent"`
		Mute    *bool `json:"mute"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Percent == nil && req.Mute == nil {
		http.Error(w, "percent or mute required", http.StatusBadRequest)
		return
	}
	if req.Percent != nil {
		s.SetStreamGain(*req.Percent)
	}
	if req.Mute != nil {
		s.SetStreamMuted(*req.Mute)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiMute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		Mute bool   `json:"mute"`
	}
	if !decode(w, r, &req) || !validKind(w, req.Kind) {
		return
	}
	if err := s.SetDeviceMute(req.Kind, req.Name, req.Mute); err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiCapture(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"source"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.SetCaptureSource(req.Source); err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiCompression(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Codec       string `json:"codec"`
		BitrateKbps int    `json:"bitrate_kbps"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Codec == "" {
		req.Codec = "none"
	}
	if err := s.SetCompression(req.Codec, req.BitrateKbps); err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func validKind(w http.ResponseWriter, kind string) bool {
	if kind != "sink" && kind != "source" {
		http.Error(w, "kind must be sink or source", http.StatusBadRequest)
		return false
	}
	return true
}

func httpErr(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
