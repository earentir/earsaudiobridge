// Package clientapp runs the EarsAudioBridge client with an optional local web UI.
package clientapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/earentir/audiobridge/internal/audio"
	"github.com/earentir/audiobridge/internal/audio/pick"
	"github.com/earentir/audiobridge/internal/codec"
	"github.com/earentir/audiobridge/internal/config"
	"github.com/earentir/audiobridge/internal/procmon"
	"github.com/earentir/audiobridge/internal/proto"
	"github.com/earentir/audiobridge/internal/stats"
)

const micFrameMS = 20

// App is the long-lived client process.
type App struct {
	cfg *config.ClientConfig

	mu          sync.RWMutex
	connected   bool
	connecting  bool
	connectedAt time.Time
	lastError   string
	backend     string
	audioFormat string
	micFormat   string
	codecName   string
	codecKbps   int

	playbackStats stats.Stream
	micStats      stats.Stream
	playbackNet   *stats.NetTiming
	proc          *procmon.Monitor

	clockOffsetNS int64
	clock         proto.ClockView

	gainMu        sync.RWMutex
	playbackGain  float64
	playbackMuted bool
	micGain       float64
	micMuted      bool

	cancelSession context.CancelFunc
	reconnect     chan struct{}

	version string
}

// Run loads config and runs until interrupted.
func Run(cfgPath, version string, withTray bool) error {
	cfg, err := config.LoadClient(cfgPath)
	if err != nil {
		return err
	}
	if !cfg.Playback.Enabled && !cfg.Mic.Enabled {
		return fmt.Errorf("nothing to do: enable playback and/or mic in the config")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := &App{
		cfg:           cfg,
		reconnect:     make(chan struct{}, 1),
		proc:          procmon.New(),
		version:       version,
		playbackGain:  float64(cfg.Playback.GainPercent) / 100,
		playbackMuted: cfg.Playback.Muted,
		micGain:       float64(cfg.Mic.GainPercent) / 100,
		micMuted:      cfg.Mic.Muted,
	}
	go func() {
		if err := app.serveWeb(); err != nil {
			log.Fatalf("web ui: %v", err)
		}
	}()
	if withTray {
		return runWithTray(ctx, stop, app)
	}
	return app.loop(ctx)
}

func (a *App) loop(ctx context.Context) error {
	for {
		a.setSessionState(true, false, "")
		sessionCtx, cancel := context.WithCancel(ctx)
		a.mu.Lock()
		a.cancelSession = cancel
		a.mu.Unlock()

		err := a.runSession(sessionCtx)
		cancel()

		a.setSessionState(false, false, errString(err))

		if ctx.Err() != nil {
			return nil
		}

		wait := time.Duration(a.cfg.ReconnectSeconds) * time.Second
		select {
		case <-ctx.Done():
			return nil
		case <-a.reconnect:
			log.Print("reconnecting (settings changed)")
		case <-time.After(wait):
			log.Printf("reconnecting to %s", a.cfg.Server)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) setSessionState(connecting, connected bool, lastErr string) {
	a.mu.Lock()
	a.connecting = connecting
	if connected && !a.connected {
		a.connectedAt = time.Now()
	}
	if !connected {
		a.connectedAt = time.Time{}
	}
	a.connected = connected
	a.lastError = lastErr
	a.mu.Unlock()
}

func (a *App) triggerReconnect() {
	a.mu.Lock()
	cancel := a.cancelSession
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case a.reconnect <- struct{}{}:
	default:
	}
}

func (a *App) runSession(ctx context.Context) error {
	conn, err := net.Dial("tcp", a.cfg.Server)
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	a.setSessionState(false, true, "")
	log.Printf("connected to %s", a.cfg.Server)

	a.mu.Lock()
	a.clock = proto.ClockView{}
	a.clockOffsetNS = 0
	a.mu.Unlock()

	var flags byte
	if a.cfg.Playback.Enabled {
		flags |= proto.FlagWantsAudio
	}
	if a.cfg.Mic.Enabled {
		flags |= proto.FlagSendsMic
	}
	t0 := time.Now()
	if _, err := conn.Write(proto.BuildHandshake(flags, t0)); err != nil {
		return err
	}

	t, payload, err := proto.ReadFrame(conn)
	if err != nil {
		return err
	}
	if t != proto.TypeHello {
		return fmt.Errorf("expected hello frame, got type %d", t)
	}
	t3 := time.Now()
	var hello proto.Hello
	if err := json.Unmarshal(payload, &hello); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	if hello.Time != nil {
		offsetNS, rttNS := proto.ComputeClockSync(
			hello.Time.ClientSendNS,
			hello.Time.ServerRecvNS,
			hello.Time.ServerSendNS,
			t3.UnixNano(),
		)
		clock := proto.ClockViewFromSync(
			hello.Time.ClientSendNS,
			hello.Time.ServerRecvNS,
			hello.Time.ServerSendNS,
			t3.UnixNano(),
			offsetNS,
			rttNS,
		)
		a.mu.Lock()
		a.clockOffsetNS = offsetNS
		a.clock = clock
		a.mu.Unlock()
		log.Printf("clock sync: offset %+.1f ms, RTT %.1f ms", clock.OffsetMS, clock.RTTMS)

		ack, err := json.Marshal(proto.TimeAck{ClientRecvNS: t3.UnixNano()})
		if err != nil {
			return err
		}
		if err := proto.WriteFrame(conn, proto.TypeTimeSync, ack); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.audioFormat = formatLabel(hello.Audio.Rate, hello.Audio.Channels)
	a.micFormat = formatLabel(hello.Mic.Rate, hello.Mic.Channels)
	a.codecName = hello.Codec.Name
	a.codecKbps = hello.Codec.BitrateKbps
	a.mu.Unlock()
	log.Printf("stream format: %d Hz %d ch, mic: %d Hz %d ch, codec: %s %d kbit/s",
		hello.Audio.Rate, hello.Audio.Channels, hello.Mic.Rate, hello.Mic.Channels,
		hello.Codec.Name, hello.Codec.BitrateKbps)

	streamCodec, err := newStreamCodec(hello)
	if err != nil {
		return err
	}
	a.playbackNet = stats.NewNetTiming(time.Duration(wireFrameMS(hello.Codec.Name)) * time.Millisecond)

	var player io.WriteCloser
	if a.cfg.Playback.Enabled {
		w, stop, backend, err := pick.ClientPlayer(a.cfg.Backend, a.cfg.Playback.Device,
			audio.Format{Rate: hello.Audio.Rate, Channels: hello.Audio.Channels},
			a.cfg.Playback.LatencyMS)
		if err != nil {
			return err
		}
		defer stop()
		player = w
		a.mu.Lock()
		a.backend = backend
		a.mu.Unlock()
		log.Printf("playback via %s backend", backend)
	}

	var writeMu sync.Mutex
	if a.cfg.Mic.Enabled {
		micFormat := audio.Format{Rate: hello.Mic.Rate, Channels: hello.Mic.Channels}
		rec, stop, backend, err := pick.ClientRecorder(a.cfg.Backend, a.cfg.Mic.Device, micFormat)
		if err != nil {
			return err
		}
		defer stop()
		if a.backend == "" {
			a.mu.Lock()
			a.backend = backend
			a.mu.Unlock()
		}
		log.Printf("mic capture via %s backend", backend)

		frameBytes := micFormat.Rate * micFormat.Channels * 2 * micFrameMS / 1000
		go func() {
			buf := make([]byte, frameBytes)
			for {
				if _, err := io.ReadFull(rec, buf); err != nil {
					if ctx.Err() == nil {
						log.Printf("mic capture ended: %v", err)
					}
					conn.Close()
					return
				}
				writeMu.Lock()
				err := proto.WriteFrame(conn, proto.TypeMic, proto.WithSendTime(time.Now(), buf))
				writeMu.Unlock()
				if err != nil {
					return
				}
				a.applyMicGain(buf)
				a.micStats.AddFrame(buf, len(buf))
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		t, payload, err := proto.ReadFrame(conn)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		switch t {
		case proto.TypeAudio:
			if player == nil {
				continue
			}
			now := time.Now()
			sendAt, wire, ok := proto.ParseSendTime(payload)
			if !ok {
				wire = payload
				if a.playbackNet != nil {
					a.playbackNet.RecordArrival(now)
				}
			} else if a.playbackNet != nil {
				a.mu.RLock()
				offset := a.clockOffsetNS
				a.mu.RUnlock()
				a.playbackNet.RecordTimed(sendAt, now, offset, true)
			}
			pcm, err := streamCodec.Decode(wire)
			if err != nil {
				return fmt.Errorf("decode: %w", err)
			}
			a.applyPlaybackGain(pcm)
			a.playbackStats.AddFrame(pcm, len(wire))
			if _, err := player.Write(pcm); err != nil {
				return fmt.Errorf("playback: %w", err)
			}
		case proto.TypeReconfig:
			var h proto.Hello
			if err := json.Unmarshal(payload, &h); err != nil {
				return fmt.Errorf("reconfig: %w", err)
			}
			streamCodec, err = newStreamCodec(h)
			if err != nil {
				return err
			}
			a.mu.Lock()
			a.codecName = h.Codec.Name
			a.codecKbps = h.Codec.BitrateKbps
			a.mu.Unlock()
			a.playbackNet = stats.NewNetTiming(time.Duration(wireFrameMS(h.Codec.Name)) * time.Millisecond)
			log.Printf("stream codec changed to %s %d kbit/s", h.Codec.Name, h.Codec.BitrateKbps)
		}
	}
}

func newStreamCodec(hello proto.Hello) (codec.Codec, error) {
	return codec.New(codec.Info{
		Name:        hello.Codec.Name,
		BitrateKbps: hello.Codec.BitrateKbps,
	}, audio.Format{Rate: hello.Audio.Rate, Channels: hello.Audio.Channels})
}

func (a *App) SetPlaybackGain(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 150 {
		percent = 150
	}
	a.gainMu.Lock()
	a.playbackGain = float64(percent) / 100
	a.cfg.Playback.GainPercent = percent
	a.gainMu.Unlock()
}

func (a *App) SetPlaybackMuted(mute bool) {
	a.gainMu.Lock()
	a.playbackMuted = mute
	a.cfg.Playback.Muted = mute
	a.gainMu.Unlock()
}

func (a *App) SetMicGain(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 150 {
		percent = 150
	}
	a.gainMu.Lock()
	a.micGain = float64(percent) / 100
	a.cfg.Mic.GainPercent = percent
	a.gainMu.Unlock()
}

func (a *App) SetMicMuted(mute bool) {
	a.gainMu.Lock()
	a.micMuted = mute
	a.cfg.Mic.Muted = mute
	a.gainMu.Unlock()
}

func (a *App) applyPlaybackGain(pcm []byte) {
	a.gainMu.RLock()
	gain := a.playbackGain
	muted := a.playbackMuted
	a.gainMu.RUnlock()
	if muted {
		for i := range pcm {
			pcm[i] = 0
		}
		return
	}
	audio.ApplyGain(pcm, gain)
}

func (a *App) applyMicGain(pcm []byte) {
	a.gainMu.RLock()
	gain := a.micGain
	muted := a.micMuted
	a.gainMu.RUnlock()
	if muted {
		for i := range pcm {
			pcm[i] = 0
		}
		return
	}
	audio.ApplyGain(pcm, gain)
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

func (a *App) liveClock() proto.ClockView {
	a.mu.RLock()
	c := a.clock
	offset := a.clockOffsetNS
	a.mu.RUnlock()
	if !c.Synced {
		return c
	}
	now := time.Now()
	c.ServerTimeNS = proto.EstimateServerNow(now, offset).UnixNano()
	c.ClientTimeNS = now.UnixNano()
	return c
}

func (a *App) snapshot() stateResponse {
	a.mu.RLock()
	status := "disconnected"
	switch {
	case a.connected:
		status = "connected"
	case a.connecting:
		status = "connecting"
	}
	playbackGain := a.cfg.Playback.GainPercent
	playbackMuted := a.cfg.Playback.Muted
	micGain := a.cfg.Mic.GainPercent
	micMuted := a.cfg.Mic.Muted
	backendCfg := a.cfg.Backend
	playbackDev := a.cfg.Playback.Device
	micDev := a.cfg.Mic.Device
	playbackEnabled := a.cfg.Playback.Enabled
	micEnabled := a.cfg.Mic.Enabled
	out := stateResponse{
		Version:         a.version,
		Role:            "client",
		Status:          status,
		Server:          a.cfg.Server,
		Backend:         a.backend,
		LastError:       a.lastError,
		AudioFormat:     a.audioFormat,
		MicFormat:       a.micFormat,
		CodecName:       a.codecName,
		CodecKbps:       a.codecKbps,
		PlaybackEnabled: playbackEnabled,
		MicEnabled:      micEnabled,
		PlaybackDevice:  playbackDev,
		MicDevice:       micDev,
		LatencyMS:       a.cfg.Playback.LatencyMS,
		PlaybackGain:    playbackGain,
		PlaybackMuted:   playbackMuted,
		MicGain:         micGain,
		MicMuted:        micMuted,
		VolumeSupported: pick.ClientSupportsVolume(backendCfg),
		Stats: clientStreamStats{
			Process: a.proc.Snapshot(),
		},
		Web: webPollView{
			StatsPollMS:  a.cfg.Web.StatsPollMS,
			PeakPollMS:   a.cfg.Web.PeakPollMS,
			LayoutPollMS: a.cfg.Web.LayoutPollMS,
		},
	}
	a.mu.RUnlock()

	if out.VolumeSupported && playbackEnabled {
		if v, m, err := pick.ClientDeviceVolume(backendCfg, "sink", playbackDev); err == nil {
			out.PlaybackDeviceVolume = v
			out.PlaybackDeviceMuted = m
		}
	}
	if out.VolumeSupported && micEnabled {
		if v, m, err := pick.ClientDeviceVolume(backendCfg, "source", micDev); err == nil {
			out.MicDeviceVolume = v
			out.MicDeviceMuted = m
		}
	}
	return out
}
