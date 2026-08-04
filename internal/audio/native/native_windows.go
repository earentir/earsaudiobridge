//go:build windows

// Package native, Windows implementation: pure-Go WASAPI via go-wca (COM over
// syscall, no cgo). Provides playback and capture with device selection,
// loopback capture of render devices, and the server System (loopback capture
// + mic-out into a virtual driver device such as VB-Cable).
package native

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"

	"github.com/earentir/audiobridge/internal/audio"
)

// Available reports whether the native backend is compiled in.
func Available() bool { return true }

const (
	// Shared-mode auto conversion so we can request s16le at our rate directly.
	streamFlagsConvert = wca.AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | wca.AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY
	// WASAPI buffer duration in 100 ns units (200 ms).
	bufferDuration = wca.REFERENCE_TIME(200 * 10000)
	pollInterval   = 10 * time.Millisecond
)

func pcmFormat(f audio.Format) *wca.WAVEFORMATEX {
	block := uint16(f.Channels * 2)
	return &wca.WAVEFORMATEX{
		WFormatTag:      1, // WAVE_FORMAT_PCM
		NChannels:       uint16(f.Channels),
		NSamplesPerSec:  uint32(f.Rate),
		NAvgBytesPerSec: uint32(f.Rate) * uint32(block),
		NBlockAlign:     block,
		WBitsPerSample:  16,
	}
}

type endpoint struct {
	dev  *wca.IMMDevice
	name string
	flow uint32 // wca.ERender or wca.ECapture
}

// enumerate must run on a COM-initialized thread.
func enumerate(de *wca.IMMDeviceEnumerator, flow uint32) ([]endpoint, error) {
	var dc *wca.IMMDeviceCollection
	if err := de.EnumAudioEndpoints(flow, wca.DEVICE_STATE_ACTIVE, &dc); err != nil {
		return nil, err
	}
	defer dc.Release()
	var count uint32
	if err := dc.GetCount(&count); err != nil {
		return nil, err
	}
	var eps []endpoint
	for i := uint32(0); i < count; i++ {
		var dev *wca.IMMDevice
		if err := dc.Item(i, &dev); err != nil {
			continue
		}
		eps = append(eps, endpoint{dev: dev, name: friendlyName(dev), flow: flow})
	}
	return eps, nil
}

func friendlyName(dev *wca.IMMDevice) string {
	var ps *wca.IPropertyStore
	if err := dev.OpenPropertyStore(wca.STGM_READ, &ps); err != nil {
		return ""
	}
	defer ps.Release()
	var pv wca.PROPVARIANT
	if err := ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err != nil {
		return ""
	}
	return pv.String()
}

// pickEndpoint resolves a device: empty name means the default endpoint of
// the given flow, otherwise exact then substring match across the flow.
func pickEndpoint(de *wca.IMMDeviceEnumerator, flow uint32, name string) (*wca.IMMDevice, error) {
	if name == "" {
		var dev *wca.IMMDevice
		if err := de.GetDefaultAudioEndpoint(flow, wca.EConsole, &dev); err != nil {
			return nil, fmt.Errorf("default endpoint: %w", err)
		}
		return dev, nil
	}
	eps, err := enumerate(de, flow)
	if err != nil {
		return nil, err
	}
	var found *wca.IMMDevice
	for _, ep := range eps {
		if found == nil && ep.name == name {
			found = ep.dev
			continue
		}
		ep.dev.Release()
	}
	if found != nil {
		return found, nil
	}
	eps, err = enumerate(de, flow)
	if err != nil {
		return nil, err
	}
	for _, ep := range eps {
		if found == nil && strings.Contains(strings.ToLower(ep.name), strings.ToLower(name)) {
			found = ep.dev
			continue
		}
		ep.dev.Release()
	}
	if found == nil {
		return nil, fmt.Errorf("no audio device matching %q", name)
	}
	return found, nil
}

// session owns one COM thread with an initialized IAudioClient.
type session struct {
	stop    chan struct{}
	stopped sync.Once
	done    chan struct{}
}

func (s *session) close() {
	s.stopped.Do(func() { close(s.stop) })
	<-s.done
}

// NewRecorder opens a capture stream. loopback captures a render device's
// output (system audio) instead of a microphone.
func NewRecorder(device string, f audio.Format, loopback bool) (io.ReadCloser, func(), error) {
	r := &recorder{frames: make(chan []byte, 32)}
	s := &session{stop: make(chan struct{}), done: make(chan struct{})}
	r.s = s

	ready := make(chan error, 1)
	go func() {
		defer close(s.done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r.loop(device, f, loopback, s.stop, ready)
	}()
	if err := <-ready; err != nil {
		<-s.done
		return nil, nil, err
	}
	return r, func() { r.Close() }, nil
}

type recorder struct {
	s      *session
	frames chan []byte
	rem    []byte
}

// loop sets up WASAPI on its own COM thread, reports startup on ready, then
// polls until stopped.
func (r *recorder) loop(device string, f audio.Format, loopback bool, stop chan struct{}, ready chan<- error) {
	fail := func(err error) { ready <- err }

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		fail(err)
		return
	}
	defer ole.CoUninitialize()

	var de *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
		fail(fmt.Errorf("device enumerator: %w", err))
		return
	}
	defer de.Release()

	// Loopback targets are render endpoints; plain capture uses capture endpoints.
	flow := uint32(wca.ECapture)
	var streamFlags uint32 = streamFlagsConvert
	if loopback {
		flow = wca.ERender
		streamFlags |= wca.AUDCLNT_STREAMFLAGS_LOOPBACK
	}
	dev, err := pickEndpoint(de, flow, device)
	if err != nil {
		fail(err)
		return
	}
	defer dev.Release()

	var ac *wca.IAudioClient
	if err := dev.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		fail(fmt.Errorf("audio client: %w", err))
		return
	}
	defer ac.Release()

	wfx := pcmFormat(f)
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, streamFlags, bufferDuration, 0, wfx, nil); err != nil {
		fail(fmt.Errorf("initialize capture: %w", err))
		return
	}

	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		fail(fmt.Errorf("capture client: %w", err))
		return
	}
	defer acc.Release()

	if err := ac.Start(); err != nil {
		fail(fmt.Errorf("start capture: %w", err))
		return
	}
	defer ac.Stop()

	ready <- nil

	block := int(wfx.NBlockAlign)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			close(r.frames)
			return
		case <-tick.C:
		}
		for {
			var packet uint32
			if err := acc.GetNextPacketSize(&packet); err != nil || packet == 0 {
				break
			}
			var data *byte
			var frames, flags uint32
			if err := acc.GetBuffer(&data, &frames, &flags, nil, nil); err != nil {
				break
			}
			if frames > 0 {
				n := int(frames) * block
				buf := make([]byte, n)
				// SILENT packets leave data undefined (often nil); treat as zeros.
				if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT == 0 && data != nil {
					copy(buf, unsafe.Slice(data, n))
				}
				select {
				case r.frames <- buf:
				default: // consumer too slow, drop
				}
			}
			acc.ReleaseBuffer(frames)
		}
	}
}

// Read implements io.Reader.
func (r *recorder) Read(b []byte) (int, error) {
	for len(r.rem) == 0 {
		buf, ok := <-r.frames
		if !ok {
			return 0, io.EOF
		}
		r.rem = buf
	}
	n := copy(b, r.rem)
	r.rem = r.rem[n:]
	return n, nil
}

// Close implements io.Closer.
func (r *recorder) Close() error {
	r.s.close()
	return nil
}

// NewPlayer opens a playback stream on the named (or default) render device.
func NewPlayer(device string, f audio.Format, latencyMS int) (io.WriteCloser, func(), error) {
	nFrames := latencyMS / 20
	if nFrames < 2 {
		nFrames = 2
	}
	p := &player{frames: make(chan []byte, nFrames)}
	s := &session{stop: make(chan struct{}), done: make(chan struct{})}
	p.s = s

	ready := make(chan error, 1)
	go func() {
		defer close(s.done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		p.loop(device, f, s.stop, ready)
	}()
	if err := <-ready; err != nil {
		<-s.done
		return nil, nil, err
	}
	return p, func() { p.Close() }, nil
}

type player struct {
	s      *session
	frames chan []byte
	rem    []byte
	mu     sync.Mutex
	closed bool
}

func (p *player) loop(device string, f audio.Format, stop chan struct{}, ready chan<- error) {
	fail := func(err error) { ready <- err }

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		fail(err)
		return
	}
	defer ole.CoUninitialize()

	var de *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
		fail(fmt.Errorf("device enumerator: %w", err))
		return
	}
	defer de.Release()

	dev, err := pickEndpoint(de, wca.ERender, device)
	if err != nil {
		fail(err)
		return
	}
	defer dev.Release()

	var ac *wca.IAudioClient
	if err := dev.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		fail(fmt.Errorf("audio client: %w", err))
		return
	}
	defer ac.Release()

	wfx := pcmFormat(f)
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, streamFlagsConvert, bufferDuration, 0, wfx, nil); err != nil {
		fail(fmt.Errorf("initialize playback: %w", err))
		return
	}

	var bufFrames uint32
	if err := ac.GetBufferSize(&bufFrames); err != nil {
		fail(err)
		return
	}
	var arc *wca.IAudioRenderClient
	if err := ac.GetService(wca.IID_IAudioRenderClient, &arc); err != nil {
		fail(fmt.Errorf("render client: %w", err))
		return
	}
	defer arc.Release()

	if err := ac.Start(); err != nil {
		fail(fmt.Errorf("start playback: %w", err))
		return
	}
	defer ac.Stop()

	ready <- nil

	block := int(wfx.NBlockAlign)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		var padding uint32
		if err := ac.GetCurrentPadding(&padding); err != nil {
			continue
		}
		writable := int(bufFrames - padding)
		for writable > 0 {
			if len(p.rem) == 0 {
				select {
				case buf := <-p.frames:
					p.rem = buf
				default:
				}
				if len(p.rem) == 0 {
					break
				}
			}
			frames := len(p.rem) / block
			if frames > writable {
				frames = writable
			}
			if frames == 0 {
				break
			}
			var data *byte
			if err := arc.GetBuffer(uint32(frames), &data); err != nil {
				break
			}
			n := frames * block
			copy(unsafe.Slice(data, n), p.rem[:n])
			p.rem = p.rem[n:]
			arc.ReleaseBuffer(uint32(frames), 0)
			writable -= frames
		}
	}
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
	p.mu.Unlock()
	p.s.close()
	return nil
}

// System is the server-side native backend on Windows: WASAPI loopback
// capture plus playback into a virtual driver device (e.g. VB-Cable) for the
// mic return path.
type System struct {
	cfg audio.SetupConfig
}

// NewSystem returns the Windows native server backend.
func NewSystem() *System { return &System{} }

// Name implements audio.System.
func (s *System) Name() string { return "native" }

// Setup implements audio.System. Devices cannot be created from userspace on
// Windows, so this only validates configuration.
func (s *System) Setup(cfg audio.SetupConfig) error {
	s.cfg = cfg
	if cfg.MicEnabled && cfg.NativeMicOutDevice == "" {
		return fmt.Errorf("native backend needs native.mic_out_device for the mic return path (e.g. VB-Cable \"CABLE Input\")")
	}
	return nil
}

// DefaultCaptureTarget implements audio.System. Empty means WASAPI loopback
// of the default output.
func (s *System) DefaultCaptureTarget() string { return s.cfg.NativeCaptureDevice }

// StartCapture implements audio.System.
func (s *System) StartCapture(target string, f audio.Format) (io.ReadCloser, func(), error) {
	return NewRecorder(target, f, true)
}

// StartMicOut implements audio.System.
func (s *System) StartMicOut(f audio.Format) (io.WriteCloser, func(), error) {
	return NewPlayer(s.cfg.NativeMicOutDevice, f, 60)
}

// Sinks implements audio.System.
func (s *System) Sinks() ([]audio.Device, error) { return listEndpoints(wca.ERender) }

// Sources implements audio.System.
func (s *System) Sources() ([]audio.Device, error) { return listEndpoints(wca.ECapture) }

func listEndpoints(flow uint32) ([]audio.Device, error) {
	type result struct {
		devs []audio.Device
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
			ch <- result{nil, err}
			return
		}
		defer ole.CoUninitialize()
		var de *wca.IMMDeviceEnumerator
		if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
			ch <- result{nil, err}
			return
		}
		defer de.Release()
		eps, err := enumerate(de, flow)
		if err != nil {
			ch <- result{nil, err}
			return
		}
		var devs []audio.Device
		for _, ep := range eps {
			devs = append(devs, audio.Device{Name: ep.name})
			ep.dev.Release()
		}
		ch <- result{devs, nil}
	}()
	r := <-ch
	return r.devs, r.err
}

// SupportsVolume implements audio.System. Endpoint volume is not wired up in
// the pure-Go build, use the OS mixer.
func (s *System) SupportsVolume() bool { return false }

// SetVolume implements audio.System.
func (s *System) SetVolume(string, string, int) error {
	return fmt.Errorf("volume control is not supported on the native backend")
}

// SetMute implements audio.System.
func (s *System) SetMute(string, string, bool) error {
	return fmt.Errorf("mute control is not supported on the native backend")
}

// Cleanup implements audio.System.
func (s *System) Cleanup() error { return nil }

// PlaybackDevices returns WASAPI render endpoints.
func PlaybackDevices() ([]audio.Device, error) {
	devs, err := listEndpoints(wca.ERender)
	if err != nil {
		return nil, err
	}
	out := make([]audio.Device, 0, len(devs)+1)
	out = append(out, audio.Device{Name: "", Description: "System default"})
	return append(out, devs...), nil
}

// CaptureDevices returns WASAPI capture endpoints.
func CaptureDevices() ([]audio.Device, error) {
	devs, err := listEndpoints(wca.ECapture)
	if err != nil {
		return nil, err
	}
	out := make([]audio.Device, 0, len(devs)+1)
	out = append(out, audio.Device{Name: "", Description: "System default"})
	return append(out, devs...), nil
}
