//go:build darwin

// Package native, macOS implementation: playback only, via oto v3 which talks
// to CoreAudio through purego (no cgo). Capture and the server role are not
// available in the pure-Go build — there is no established cgo-free CoreAudio
// capture path.
package native

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/earentir/audiobridge/internal/audio"
)

// Available reports whether the native backend is compiled in.
func Available() bool { return true }

var (
	otoOnce sync.Once
	otoCtx  *oto.Context
	otoErr  error
)

// NewPlayer opens playback on the default output device. oto does not expose
// device selection, so a configured device name is rejected rather than
// silently ignored.
func NewPlayer(device string, f audio.Format, latencyMS int) (io.WriteCloser, func(), error) {
	if device != "" {
		return nil, nil, fmt.Errorf("device selection is not supported on macOS, leave playback.device empty (default output)")
	}

	// oto allows exactly one context per process.
	otoOnce.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   f.Rate,
			ChannelCount: f.Channels,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   time.Duration(latencyMS) * time.Millisecond,
		})
		if err != nil {
			otoErr = err
			return
		}
		<-ready
		otoCtx = ctx
	})
	if otoErr != nil {
		return nil, nil, fmt.Errorf("oto: %w", otoErr)
	}

	nFrames := latencyMS / 20
	if nFrames < 2 {
		nFrames = 2
	}
	p := &player{frames: make(chan []byte, nFrames)}
	p.oto = otoCtx.NewPlayer(p)
	p.oto.Play()
	return p, func() { p.Close() }, nil
}

// player adapts our push model to oto's pull model: Write queues frames,
// Read (called by oto) drains them, blocking briefly on underrun.
type player struct {
	oto    *oto.Player
	frames chan []byte
	rem    []byte
	mu     sync.Mutex
	closed bool
}

// Read implements io.Reader for oto.
func (p *player) Read(b []byte) (int, error) {
	if len(p.rem) == 0 {
		select {
		case buf, ok := <-p.frames:
			if !ok {
				return 0, io.EOF
			}
			p.rem = buf
		case <-time.After(20 * time.Millisecond):
			// Underrun: feed silence so oto keeps the stream alive.
			n := len(b)
			if n > 3840 {
				n = 3840
			}
			for i := 0; i < n; i++ {
				b[i] = 0
			}
			return n, nil
		}
	}
	n := copy(b, p.rem)
	p.rem = p.rem[n:]
	return n, nil
}

// Write implements io.Writer; drops the oldest frame when full to keep
// latency bounded.
func (p *player) Write(b []byte) (int, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	p.mu.Unlock()

	buf := make([]byte, len(b))
	copy(buf, b)
	select {
	case p.frames <- buf:
	default:
		select {
		case <-p.frames:
		default:
		}
		select {
		case p.frames <- buf:
		default:
		}
	}
	return len(b), nil
}

// Close implements io.Closer.
func (p *player) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.frames)
	p.mu.Unlock()
	return p.oto.Close()
}

var errDarwinCapture = fmt.Errorf("audio capture is not available in the pure-Go macOS build")

// NewRecorder is unavailable on macOS in the pure-Go build.
func NewRecorder(string, audio.Format, bool) (io.ReadCloser, func(), error) {
	return nil, nil, errDarwinCapture
}

// System is unavailable on macOS: the server role needs capture.
type System struct{}

// NewSystem returns the macOS server backend stub.
func NewSystem() *System { return &System{} }

// Name implements audio.System.
func (s *System) Name() string { return "native" }

// Setup implements audio.System.
func (s *System) Setup(audio.SetupConfig) error {
	return fmt.Errorf("server role is not available on macOS in the pure-Go build (no capture); run the server on Linux or Windows")
}

// DefaultCaptureTarget implements audio.System.
func (s *System) DefaultCaptureTarget() string { return "" }

// StartCapture implements audio.System.
func (s *System) StartCapture(string, audio.Format) (io.ReadCloser, func(), error) {
	return nil, nil, errDarwinCapture
}

// StartMicOut implements audio.System.
func (s *System) StartMicOut(audio.Format) (io.WriteCloser, func(), error) {
	return nil, nil, errDarwinCapture
}

// Sinks implements audio.System.
func (s *System) Sinks() ([]audio.Device, error) { return nil, errDarwinCapture }

// Sources implements audio.System.
func (s *System) Sources() ([]audio.Device, error) { return nil, errDarwinCapture }

// SupportsVolume implements audio.System.
func (s *System) SupportsVolume() bool { return false }

// SetVolume implements audio.System.
func (s *System) SetVolume(string, string, int) error { return errDarwinCapture }

// SetMute implements audio.System.
func (s *System) SetMute(string, string, bool) error { return errDarwinCapture }

// Cleanup implements audio.System.
func (s *System) Cleanup() error { return nil }

// PlaybackDevices returns the default output only on macOS.
func PlaybackDevices() ([]audio.Device, error) {
	return []audio.Device{{Name: "", Description: "System default"}}, nil
}

// CaptureDevices is unavailable on macOS in the pure-Go build.
func CaptureDevices() ([]audio.Device, error) { return nil, errDarwinCapture }
