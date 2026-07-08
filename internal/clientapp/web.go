package clientapp

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/earentir/audiobridge/internal/audio"
	"github.com/earentir/audiobridge/internal/audio/pick"
	"github.com/earentir/audiobridge/internal/procmon"
	"github.com/earentir/audiobridge/internal/proto"
	"github.com/earentir/audiobridge/internal/stats"
)

//go:embed web
var webFS embed.FS

type clientStreamStats struct {
	Process           procmon.Snapshot  `json:"process"`
	Playback          stats.Snapshot    `json:"playback"`
	Mic               stats.Snapshot    `json:"mic"`
	PlaybackNet       stats.NetSnapshot `json:"playback_net"`
	Clock             proto.ClockView   `json:"clock"`
	TotalWireKbps     float64           `json:"total_wire_kbps"`
	TotalIngressKbps  float64           `json:"total_ingress_kbps"`
	TotalEgressKbps   float64           `json:"total_egress_kbps"`
	ConnectedSec      int               `json:"connected_sec"`
	WireFrameMS       int               `json:"wire_frame_ms"`
	PlaybackLatencyMS int               `json:"playback_latency_ms"`
	Server            string            `json:"server"`
}

type peakStream struct {
	Peak int `json:"peak"`
}

type clientStreamPeaks struct {
	Playback peakStream `json:"playback"`
	Mic      peakStream `json:"mic"`
}

type compressionView struct {
	Codec       string `json:"codec"`
	BitrateKbps int    `json:"bitrate_kbps"`
}

type clientStatsResponse struct {
	Status          string            `json:"status"`
	Server          string            `json:"server"`
	LastError       string            `json:"last_error"`
	Backend         string            `json:"backend"`
	AudioFormat     string            `json:"audio_format"`
	MicFormat       string            `json:"mic_format"`
	Compression     compressionView   `json:"compression"`
	PlaybackEnabled bool              `json:"playback_enabled"`
	MicEnabled      bool              `json:"mic_enabled"`
	Stats           clientStreamStats `json:"stats"`
}

type webPollView struct {
	StatsPollMS  int `json:"stats_poll_ms"`
	PeakPollMS   int `json:"peak_poll_ms"`
	LayoutPollMS int `json:"layout_poll_ms"`
}

type stateResponse struct {
	Version              string            `json:"version"`
	Role                 string            `json:"role"`
	Status               string            `json:"status"`
	Server               string            `json:"server"`
	Backend              string            `json:"backend"`
	LastError            string            `json:"last_error"`
	AudioFormat          string            `json:"audio_format"`
	MicFormat            string            `json:"mic_format"`
	CodecName            string            `json:"codec_name"`
	CodecKbps            int               `json:"codec_kbps"`
	PlaybackEnabled      bool              `json:"playback_enabled"`
	MicEnabled           bool              `json:"mic_enabled"`
	PlaybackDevice       string            `json:"playback_device"`
	MicDevice            string            `json:"mic_device"`
	LatencyMS            int               `json:"latency_ms"`
	PlaybackGain         int               `json:"playback_gain"`
	PlaybackMuted        bool              `json:"playback_muted"`
	MicGain              int               `json:"mic_gain"`
	MicMuted             bool              `json:"mic_muted"`
	VolumeSupported      bool              `json:"volume_supported"`
	PlaybackDeviceVolume int               `json:"playback_device_volume"`
	PlaybackDeviceMuted  bool              `json:"playback_device_muted"`
	MicDeviceVolume      int               `json:"mic_device_volume"`
	MicDeviceMuted       bool              `json:"mic_device_muted"`
	Stats                clientStreamStats `json:"stats"`
	Outputs              []audio.Device    `json:"outputs,omitempty"`
	Inputs               []audio.Device    `json:"inputs,omitempty"`
	Web                  webPollView       `json:"web"`
}

func (a *App) serveWeb() error {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("GET /api/state", a.apiState)
	mux.HandleFunc("GET /api/stats", a.apiStats)
	mux.HandleFunc("GET /api/peaks", a.apiPeaks)
	mux.HandleFunc("POST /api/settings", a.apiSettings)
	mux.HandleFunc("POST /api/playback-gain", a.apiPlaybackGain)
	mux.HandleFunc("POST /api/mic-gain", a.apiMicGain)
	mux.HandleFunc("POST /api/device-volume", a.apiDeviceVolume)
	mux.HandleFunc("POST /api/device-mute", a.apiDeviceMute)

	log.Printf("web ui on %s", a.cfg.Web.Listen)
	return http.ListenAndServe(a.cfg.Web.Listen, mux)
}

func (a *App) apiState(w http.ResponseWriter, r *http.Request) {
	out := a.snapshot()
	if outs, err := pick.ClientPlaybackDevices(a.cfg.Backend); err == nil {
		out.Outputs = outs
	}
	if ins, err := pick.ClientCaptureDevices(a.cfg.Backend); err == nil {
		out.Inputs = ins
	}
	writeJSON(w, out)
}

func (a *App) apiStats(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	status := "disconnected"
	switch {
	case a.connected:
		status = "connected"
	case a.connecting:
		status = "connecting"
	}
	codecName := a.codecName
	codecKbps := a.codecKbps
	out := clientStatsResponse{
		Status:      status,
		Server:      a.cfg.Server,
		LastError:   a.lastError,
		Backend:     a.backend,
		AudioFormat: a.audioFormat,
		MicFormat:   a.micFormat,
		Compression: compressionView{
			Codec:       codecName,
			BitrateKbps: codecKbps,
		},
		PlaybackEnabled: a.cfg.Playback.Enabled,
		MicEnabled:      a.cfg.Mic.Enabled,
		Stats:           a.streamStatsLocked(),
	}
	a.mu.RUnlock()
	writeJSON(w, out)
}

func (a *App) apiPeaks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]clientStreamPeaks{
		"peaks": {
			Playback: peakStream{Peak: a.playbackStats.TakePeak()},
			Mic:      peakStream{Peak: a.micStats.TakePeak()},
		},
	})
}

func (a *App) streamStatsLocked() clientStreamStats {
	playback := a.playbackStats.RateSnapshot()
	mic := a.micStats.RateSnapshot()
	wireMS := wireFrameMS(a.codecName)
	connectedSec := 0
	if a.connected && !a.connectedAt.IsZero() {
		connectedSec = int(time.Since(a.connectedAt).Seconds())
	}
	return clientStreamStats{
		Process:           a.proc.Snapshot(),
		Playback:          playback,
		Mic:               mic,
		PlaybackNet:       netSnapshot(a.playbackNet),
		Clock:             a.liveClock(),
		TotalWireKbps:     playback.BandwidthKbps + mic.BandwidthKbps,
		TotalIngressKbps:  playback.BandwidthKbps,
		TotalEgressKbps:   mic.BandwidthKbps,
		ConnectedSec:      connectedSec,
		WireFrameMS:       wireMS,
		PlaybackLatencyMS: a.cfg.Playback.LatencyMS,
		Server:            a.cfg.Server,
	}
}

func wireFrameMS(codec string) int {
	if codec == "flac" {
		return 80
	}
	return 20
}

func netSnapshot(n *stats.NetTiming) stats.NetSnapshot {
	if n == nil {
		return stats.NetSnapshot{}
	}
	return n.Snapshot()
}

func (a *App) apiSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlaybackEnabled *bool  `json:"playback_enabled"`
		MicEnabled      *bool  `json:"mic_enabled"`
		PlaybackDevice  string `json:"playback_device"`
		MicDevice       string `json:"mic_device"`
		LatencyMS       *int   `json:"latency_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	a.mu.Lock()
	if req.PlaybackEnabled != nil {
		a.cfg.Playback.Enabled = *req.PlaybackEnabled
	}
	if req.MicEnabled != nil {
		a.cfg.Mic.Enabled = *req.MicEnabled
	}
	a.cfg.Playback.Device = req.PlaybackDevice
	a.cfg.Mic.Device = req.MicDevice
	if req.LatencyMS != nil && *req.LatencyMS > 0 {
		a.cfg.Playback.LatencyMS = *req.LatencyMS
	}
	if !a.cfg.Playback.Enabled && !a.cfg.Mic.Enabled {
		a.mu.Unlock()
		http.Error(w, "enable playback and/or mic", http.StatusBadRequest)
		return
	}
	a.mu.Unlock()

	a.triggerReconnect()
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) apiPlaybackGain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Percent *int  `json:"percent"`
		Mute    *bool `json:"mute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Percent == nil && req.Mute == nil {
		http.Error(w, "percent or mute required", http.StatusBadRequest)
		return
	}
	if req.Percent != nil {
		a.SetPlaybackGain(*req.Percent)
	}
	if req.Mute != nil {
		a.SetPlaybackMuted(*req.Mute)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) apiMicGain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Percent *int  `json:"percent"`
		Mute    *bool `json:"mute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Percent == nil && req.Mute == nil {
		http.Error(w, "percent or mute required", http.StatusBadRequest)
		return
	}
	if req.Percent != nil {
		a.SetMicGain(*req.Percent)
	}
	if req.Mute != nil {
		a.SetMicMuted(*req.Mute)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) apiDeviceVolume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role    string `json:"role"`
		Percent int    `json:"percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	kind, name, err := a.deviceRole(req.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := pick.ClientSetVolume(a.cfg.Backend, kind, name, req.Percent); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) apiDeviceMute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
		Mute bool   `json:"mute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	kind, name, err := a.deviceRole(req.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := pick.ClientSetMute(a.cfg.Backend, kind, name, req.Mute); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *App) deviceRole(role string) (kind, name string, err error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	switch role {
	case "playback":
		return "sink", a.cfg.Playback.Device, nil
	case "mic":
		return "source", a.cfg.Mic.Device, nil
	default:
		return "", "", errBadRole
	}
}

var errBadRole = &badRoleError{}

type badRoleError struct{}

func (e *badRoleError) Error() string { return "role must be playback or mic" }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
