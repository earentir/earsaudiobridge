// Package server implements the EarsAudioBridge server: virtual device setup via
// the selected audio backend, capture and broadcast, the return mic path and
// the web UI.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/earentir/audiobridge/internal/audio"
	"github.com/earentir/audiobridge/internal/codec"
	"github.com/earentir/audiobridge/internal/config"
	"github.com/earentir/audiobridge/internal/procmon"
	"github.com/earentir/audiobridge/internal/proto"
	"github.com/earentir/audiobridge/internal/stats"
)

const frameMS = 20

// Server holds all runtime state.
type Server struct {
	cfg *config.ServerConfig
	sys audio.System

	mu      sync.Mutex
	clients map[*client]struct{}

	codecMu     sync.RWMutex
	streamCodec codec.Codec

	capMu      sync.Mutex
	capReader  io.ReadCloser
	capStop    func()
	captureSrc string

	micMu   sync.Mutex
	micOut  io.WriteCloser
	micStop func()

	captureStats stats.Stream
	micStats     stats.Stream
	micNet       *stats.NetTiming
	proc         *procmon.Monitor

	gainMu      sync.RWMutex
	streamGain  float64 // 1.0 = 100%, applied to captured audio before broadcast
	streamMuted bool

	version string
}

type client struct {
	conn          net.Conn
	writeMu       sync.Mutex
	send          chan []byte
	wantsAudio    bool
	sendsMic      bool
	stats         stats.Stream
	micNet        *stats.NetTiming
	connectedAt   time.Time
	micBytesRx    uint64
	queuePeak     int
	clockOffsetNS int64 // server minus client
	clock         proto.ClockView
}

// New creates a server for the given config and backend.
func New(cfg *config.ServerConfig, sys audio.System, version string) *Server {
	gain := float64(cfg.Stream.GainPercent) / 100
	return &Server{
		cfg:         cfg,
		sys:         sys,
		clients:     map[*client]struct{}{},
		streamGain:  gain,
		streamMuted: cfg.Stream.Muted,
		proc:        procmon.New(),
		version:     version,
		micNet:      stats.NewNetTiming(frameMS * time.Millisecond),
	}
}

// Backend returns the active backend name.
func (s *Server) Backend() string { return s.sys.Name() }

// System returns the active audio backend.
func (s *Server) System() audio.System { return s.sys }

// Setup creates the virtual devices and starts capturing.
func (s *Server) Setup() error {
	if err := s.initCodec(); err != nil {
		return err
	}
	err := s.sys.Setup(audio.SetupConfig{
		SinkName:            s.cfg.Audio.SinkName,
		SinkDescription:     s.cfg.Audio.SinkDescription,
		MicEnabled:          s.cfg.Mic.Enabled,
		MicSinkName:         s.cfg.Mic.SinkName,
		MicSourceName:       s.cfg.Mic.SourceName,
		MicSourceDesc:       s.cfg.Mic.SourceDescription,
		NativeCaptureDevice: s.cfg.Native.CaptureDevice,
		NativeMicOutDevice:  s.cfg.Native.MicOutDevice,
	})
	if err != nil {
		return err
	}
	log.Printf("virtual audio devices ready")

	if s.cfg.Mic.Enabled {
		micFormat := audio.Format{Rate: s.cfg.Mic.SampleRate, Channels: s.cfg.Mic.Channels}
		w, stop, err := s.sys.StartMicOut(micFormat)
		if err != nil {
			return fmt.Errorf("mic out: %w", err)
		}
		s.micOut, s.micStop = w, stop
		log.Printf("mic return path open")
	}

	target := s.sys.DefaultCaptureTarget()
	if s.cfg.Stream.CaptureSource != "" {
		target = s.cfg.Stream.CaptureSource
	}
	return s.SetCaptureSource(target)
}

func (s *Server) initCodec() error {
	f := audio.Format{Rate: s.cfg.Audio.SampleRate, Channels: s.cfg.Audio.Channels}
	c, err := codec.New(codec.Info{
		Name:        s.cfg.Compression.Codec,
		BitrateKbps: s.cfg.Compression.BitrateKbps,
	}, f)
	if err != nil {
		return err
	}
	s.codecMu.Lock()
	s.streamCodec = c
	s.codecMu.Unlock()
	return nil
}

func (s *Server) audioFormat() audio.Format {
	return audio.Format{Rate: s.cfg.Audio.SampleRate, Channels: s.cfg.Audio.Channels}
}

func (s *Server) hello() proto.Hello {
	s.codecMu.RLock()
	info := s.streamCodec.Info()
	s.codecMu.RUnlock()
	return proto.Hello{
		Audio: proto.Format{Rate: s.cfg.Audio.SampleRate, Channels: s.cfg.Audio.Channels},
		Mic:   proto.Format{Rate: s.cfg.Mic.SampleRate, Channels: s.cfg.Mic.Channels},
		Codec: proto.CodecInfo{Name: info.Name, BitrateKbps: info.BitrateKbps},
	}
}

// Compression returns the active stream codec settings.
func (s *Server) Compression() codec.Info {
	s.codecMu.RLock()
	defer s.codecMu.RUnlock()
	return s.streamCodec.Info()
}

// SetCompression switches the outbound codec and notifies connected clients.
func (s *Server) SetCompression(name string, bitrateKbps int) error {
	c, err := codec.New(codec.Info{Name: name, BitrateKbps: bitrateKbps}, s.audioFormat())
	if err != nil {
		return err
	}
	s.codecMu.Lock()
	s.streamCodec = c
	s.cfg.Compression.Codec = name
	s.cfg.Compression.BitrateKbps = c.Info().BitrateKbps
	s.codecMu.Unlock()
	s.captureStats.Reset()
	s.mu.Lock()
	for cl := range s.clients {
		cl.stats.Reset()
	}
	s.mu.Unlock()
	if c.Info().BitrateKbps > 0 {
		log.Printf("stream codec: %s %d kbit/s", name, c.Info().BitrateKbps)
	} else {
		log.Printf("stream codec: %s", name)
	}
	return s.sendReconfig()
}

// StreamGainPercent returns the software gain applied to the outbound stream.
func (s *Server) StreamGainPercent() int {
	s.gainMu.RLock()
	defer s.gainMu.RUnlock()
	return int(s.streamGain*100 + 0.5)
}

// StreamMuted reports whether the outbound stream is muted.
func (s *Server) StreamMuted() bool {
	s.gainMu.RLock()
	defer s.gainMu.RUnlock()
	return s.streamMuted
}

// SetStreamGain sets the software gain applied to audio sent to clients.
func (s *Server) SetStreamGain(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 150 {
		percent = 150
	}
	s.gainMu.Lock()
	s.streamGain = float64(percent) / 100
	s.cfg.Stream.GainPercent = percent
	s.gainMu.Unlock()
	log.Printf("client stream volume: %d%%", percent)
}

// SetStreamMuted mutes or unmutes audio sent to clients.
func (s *Server) SetStreamMuted(mute bool) {
	s.gainMu.Lock()
	s.streamMuted = mute
	s.cfg.Stream.Muted = mute
	s.gainMu.Unlock()
	log.Printf("client stream mute: %v", mute)
}

func (s *Server) applyStreamGain(pcm []byte) {
	s.gainMu.RLock()
	gain := s.streamGain
	muted := s.streamMuted
	s.gainMu.RUnlock()
	if muted {
		for i := range pcm {
			pcm[i] = 0
		}
		return
	}
	audio.ApplyGain(pcm, gain)
}

// SetDeviceVolume sets PipeWire/Pulse volume on the host only.
func (s *Server) SetDeviceVolume(kind, name string, percent int) error {
	return s.sys.SetVolume(kind, name, percent)
}

// SetDeviceMute sets PipeWire/Pulse mute on the host only.
func (s *Server) SetDeviceMute(kind, name string, mute bool) error {
	return s.sys.SetMute(kind, name, mute)
}

func (s *Server) sendReconfig() error {
	payload, err := json.Marshal(s.hello())
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		c.writeMu.Lock()
		err := proto.WriteFrame(c.conn, proto.TypeReconfig, payload)
		c.writeMu.Unlock()
		if err != nil {
			c.conn.Close()
		}
	}
	return nil
}

func (s *Server) encode(pcm []byte) ([]byte, error) {
	s.codecMu.RLock()
	c := s.streamCodec
	s.codecMu.RUnlock()
	return c.Encode(pcm)
}

// Shutdown stops streams and optionally removes the created devices.
func (s *Server) Shutdown() {
	s.capMu.Lock()
	s.stopCaptureLocked()
	s.capMu.Unlock()

	s.micMu.Lock()
	if s.micStop != nil {
		s.micStop()
		s.micOut, s.micStop = nil, nil
	}
	s.micMu.Unlock()

	if s.cfg.CleanupOnExit {
		if err := s.sys.Cleanup(); err != nil {
			log.Printf("cleanup: %v", err)
		}
	}
}

// CaptureSource returns the target currently being streamed.
func (s *Server) CaptureSource() string {
	s.capMu.Lock()
	defer s.capMu.Unlock()
	return s.captureSrc
}

// SetCaptureSource restarts capture on a different target.
func (s *Server) SetCaptureSource(target string) error {
	s.capMu.Lock()
	defer s.capMu.Unlock()
	s.stopCaptureLocked()

	f := s.audioFormat()
	r, stop, err := s.sys.StartCapture(target, f)
	if err != nil {
		return err
	}
	s.capReader, s.capStop, s.captureSrc = r, stop, target
	s.cfg.Stream.CaptureSource = target
	log.Printf("capturing %q (%d Hz, %d ch, %s backend)", target, f.Rate, f.Channels, s.sys.Name())

	frameBytes := f.Rate * f.Channels * 2 * frameMS / 1000
	go s.pump(r, frameBytes)
	return nil
}

func (s *Server) stopCaptureLocked() {
	if s.capStop != nil {
		s.capStop()
		s.capReader, s.capStop = nil, nil
	}
}

func (s *Server) pump(r io.Reader, frameBytes int) {
	var pendingPCM []byte
	for {
		buf := make([]byte, frameBytes)
		if _, err := io.ReadFull(r, buf); err != nil {
			s.capMu.Lock()
			current := s.capReader == r
			s.capMu.Unlock()
			if current {
				log.Printf("capture ended: %v", err)
			}
			return
		}
		s.applyStreamGain(buf)
		pendingPCM = append(pendingPCM, buf...)
		wire, err := s.encode(buf)
		if errors.Is(err, codec.ErrBuffering) {
			continue
		}
		if err != nil {
			log.Printf("encode: %v", err)
			pendingPCM = pendingPCM[:0]
			continue
		}
		pcmBatch := append([]byte(nil), pendingPCM...)
		pendingPCM = pendingPCM[:0]
		s.captureStats.AddFrame(pcmBatch, len(wire))
		s.broadcast(wire, pcmBatch)
	}
}

func (s *Server) broadcast(wire, pcm []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if !c.wantsAudio {
			continue
		}
		select {
		case c.send <- wire:
			c.stats.AddFrame(pcm, len(wire))
			if q := len(c.send); q > c.queuePeak {
				c.queuePeak = q
			}
		default:
			c.stats.AddDropped(1)
			s.captureStats.AddDropped(1)
		}
	}
}

// ClientCount returns the number of connected clients.
func (s *Server) ClientCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

type clientStats struct {
	Addr         string            `json:"addr"`
	Audio        bool              `json:"audio"`
	SendsMic     bool              `json:"sends_mic"`
	QueueDepth   int               `json:"queue_depth"`
	QueuePeak    int               `json:"queue_peak"`
	LatencyMS    int               `json:"latency_ms"`
	ConnectedSec int               `json:"connected_sec"`
	MicBytesRx   uint64            `json:"mic_bytes_rx"`
	MicNet       stats.NetSnapshot `json:"mic_net,omitempty"`
	Clock        proto.ClockView   `json:"clock,omitempty"`
	Stream       stats.Snapshot    `json:"stream"`
}

// StreamStats holds server-side audio counters for the web UI.
type StreamStats struct {
	Process         procmon.Snapshot  `json:"process"`
	Capture         stats.Snapshot    `json:"capture"`
	Mic             stats.Snapshot    `json:"mic"`
	Clients         []clientStats     `json:"clients"`
	TotalWireKbps   float64           `json:"total_wire_kbps"`
	ClientCount     int               `json:"client_count"`
	StreamListen    string            `json:"stream_listen"`
	EncodeLatencyMS int               `json:"encode_latency_ms"`
	WireFrameMS     int               `json:"wire_frame_ms"`
	MicNet          stats.NetSnapshot `json:"mic_net"`
}

// Stats returns current stream counters and short-term rates.
func (s *Server) Stats() StreamStats {
	comp := s.Compression()
	wireMS := wireFrameMS(comp.Name)
	out := StreamStats{
		Process:         s.proc.Snapshot(),
		Capture:         s.captureStats.RateSnapshot(),
		Mic:             s.micStats.RateSnapshot(),
		StreamListen:    s.cfg.Listen,
		EncodeLatencyMS: wireMS,
		WireFrameMS:     wireMS,
		MicNet:          s.micNet.Snapshot(),
	}
	s.mu.Lock()
	out.ClientCount = len(s.clients)
	for c := range s.clients {
		snap := c.stats.RateSnapshot()
		q := len(c.send)
		out.TotalWireKbps += snap.BandwidthKbps
		out.Clients = append(out.Clients, clientStats{
			Addr:         c.conn.RemoteAddr().String(),
			Audio:        c.wantsAudio,
			SendsMic:     c.sendsMic,
			QueueDepth:   q,
			QueuePeak:    c.queuePeak,
			LatencyMS:    wireMS + q*wireMS,
			ConnectedSec: int(time.Since(c.connectedAt).Seconds()),
			MicBytesRx:   c.micBytesRx,
			MicNet:       micNetSnapshot(c.micNet),
			Clock:        c.liveClock(),
			Stream:       snap,
		})
	}
	s.mu.Unlock()
	return out
}

func wireFrameMS(codec string) int {
	if codec == "flac" {
		return 80
	}
	return 20
}

func micNetSnapshot(n *stats.NetTiming) stats.NetSnapshot {
	if n == nil {
		return stats.NetSnapshot{}
	}
	return n.Snapshot()
}

func (c *client) liveClock() proto.ClockView {
	if !c.clock.Synced {
		return c.clock
	}
	now := time.Now()
	cv := c.clock
	cv.ServerTimeNS = now.UnixNano()
	cv.ClientTimeNS = proto.EstimateClientNow(now, c.clockOffsetNS).UnixNano()
	return cv
}

type peakStream struct {
	Peak int `json:"peak"`
}

type clientPeak struct {
	Addr  string `json:"addr"`
	Audio bool   `json:"audio"`
	Peak  int    `json:"peak"`
}

// StreamPeaks holds peak levels for VU meters without affecting rate stats.
type StreamPeaks struct {
	Capture peakStream   `json:"capture"`
	Mic     peakStream   `json:"mic"`
	Clients []clientPeak `json:"clients"`
}

// Peaks returns peak levels since the last peaks poll and clears them.
func (s *Server) Peaks() StreamPeaks {
	out := StreamPeaks{
		Capture: peakStream{Peak: s.captureStats.TakePeak()},
		Mic:     peakStream{Peak: s.micStats.TakePeak()},
	}
	s.mu.Lock()
	for c := range s.clients {
		out.Clients = append(out.Clients, clientPeak{
			Addr:  c.conn.RemoteAddr().String(),
			Audio: c.wantsAudio,
			Peak:  c.stats.TakePeak(),
		})
	}
	s.mu.Unlock()
	return out
}

func (s *Server) writeMic(frame []byte) {
	s.micMu.Lock()
	defer s.micMu.Unlock()
	if s.micOut != nil {
		s.micOut.Write(frame)
		s.micStats.AddFrame(frame, len(frame))
	}
}

// ListenAndServe accepts streaming clients on the configured TCP port.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return err
	}
	log.Printf("stream listener on %s", s.cfg.Listen)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	hs := make([]byte, proto.HandshakeLen)
	if _, err := io.ReadFull(conn, hs); err != nil {
		return
	}
	flags, t0, ok := proto.ParseHandshake(hs)
	if !ok {
		return
	}
	t1 := time.Now()

	hi := s.hello()
	t2 := time.Now()
	hi.Time = &proto.TimeInfo{
		ClientSendNS: t0,
		ServerRecvNS: t1.UnixNano(),
		ServerSendNS: t2.UnixNano(),
	}
	hello, err := json.Marshal(hi)
	if err != nil {
		return
	}
	if err := proto.WriteFrame(conn, proto.TypeHello, hello); err != nil {
		return
	}

	tSyncType, tSyncPayload, err := proto.ReadFrame(conn)
	if err != nil || tSyncType != proto.TypeTimeSync {
		return
	}
	var ack proto.TimeAck
	if err := json.Unmarshal(tSyncPayload, &ack); err != nil {
		return
	}
	offsetNS, rttNS := proto.ComputeClockSync(t0, t1.UnixNano(), t2.UnixNano(), ack.ClientRecvNS)
	clock := proto.ClockViewFromSync(t0, t1.UnixNano(), t2.UnixNano(), ack.ClientRecvNS, offsetNS, rttNS)
	log.Printf("client %s clock sync: offset %+.1f ms, RTT %.1f ms",
		conn.RemoteAddr(), clock.OffsetMS, clock.RTTMS)

	c := &client{
		conn:          conn,
		send:          make(chan []byte, 64),
		wantsAudio:    flags&proto.FlagWantsAudio != 0,
		sendsMic:      flags&proto.FlagSendsMic != 0,
		connectedAt:   time.Now(),
		clockOffsetNS: offsetNS,
		clock:         clock,
	}
	if c.sendsMic {
		c.micNet = stats.NewNetTiming(frameMS * time.Millisecond)
	}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	log.Printf("client %s connected (audio=%v mic=%v)", conn.RemoteAddr(), c.wantsAudio, flags&proto.FlagSendsMic != 0)

	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		log.Printf("client %s disconnected", conn.RemoteAddr())
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for frame := range c.send {
			c.writeMu.Lock()
			err := proto.WriteFrame(conn, proto.TypeAudio, proto.WithSendTime(time.Now(), frame))
			c.writeMu.Unlock()
			if err != nil {
				conn.Close()
				return
			}
		}
	}()

	for {
		t, payload, err := proto.ReadFrame(conn)
		if err != nil {
			break
		}
		if t == proto.TypeMic && s.cfg.Mic.Enabled && flags&proto.FlagSendsMic != 0 {
			now := time.Now()
			sendAt, data, ok := proto.ParseSendTime(payload)
			if !ok {
				data = payload
				s.micNet.RecordArrival(now)
				if c.micNet != nil {
					c.micNet.RecordArrival(now)
				}
			} else {
				offset := c.clockOffsetNS
				s.micNet.RecordTimed(sendAt, now, offset, false)
				if c.micNet != nil {
					c.micNet.RecordTimed(sendAt, now, offset, false)
				}
			}
			s.writeMic(data)
			c.micBytesRx += uint64(len(data))
		}
	}
	close(c.send)
	<-done
}
