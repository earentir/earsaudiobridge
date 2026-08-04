package pulseclient

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"

	"github.com/earentir/audiobridge/internal/audio"
)

// NewRecorder opens a capture stream as s16le PCM.
func NewRecorder(device string, f audio.Format, streamName string) (io.ReadCloser, func(), error) {
	c, err := openClient(streamName)
	if err != nil {
		return nil, nil, err
	}

	opts := recordOptions(f, 20)
	opts = append(opts, pulse.RecordMediaName(streamName))

	if err := applyRecordDevice(c, device, &opts); err != nil {
		c.Close()
		return nil, nil, err
	}

	pr, pw := io.Pipe()
	w := pulse.NewWriter(pw, s16Format())
	stream, err := c.NewRecord(w, opts...)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	stream.Start()

	rc := &recordCloser{
		client: c,
		stream: stream,
		pw:     pw,
		pr:     pr,
	}
	return rc, rc.stop, nil
}

// NewPlayer opens a playback stream as s16le PCM.
func NewPlayer(device string, f audio.Format, latencyMS int, streamName string) (io.WriteCloser, func(), error) {
	if latencyMS <= 0 {
		latencyMS = 20
	}
	c, err := openClient(streamName)
	if err != nil {
		return nil, nil, err
	}

	opts := playbackOptions(f, latencyMS)
	opts = append(opts, pulse.PlaybackMediaName(streamName))

	if device != "" {
		sink, err := lookupSink(c, device)
		if err != nil {
			c.Close()
			return nil, nil, err
		}
		opts = append(opts, pulse.PlaybackSink(sink))
	}

	pr, pw := io.Pipe()
	r := pulse.NewReader(pr, s16Format())
	stream, err := c.NewPlayback(r, opts...)
	if err != nil {
		c.Close()
		return nil, nil, err
	}

	wc := &playbackWriter{
		client: c,
		stream: stream,
		pr:     pr,
		pw:     pw,
	}
	return wc, wc.stop, nil
}

func recordOptions(f audio.Format, latencyMS int) []pulse.RecordOption {
	opts := []pulse.RecordOption{
		pulse.RecordSampleRate(f.Rate),
		pulse.RecordLatency(float64(latencyMS) / 1000),
	}
	if f.Channels >= 2 {
		opts = append(opts, pulse.RecordStereo)
	} else {
		opts = append(opts, pulse.RecordMono)
	}
	return opts
}

func playbackOptions(f audio.Format, latencyMS int) []pulse.PlaybackOption {
	opts := []pulse.PlaybackOption{
		pulse.PlaybackSampleRate(f.Rate),
		pulse.PlaybackLatency(float64(latencyMS) / 1000),
	}
	if f.Channels >= 2 {
		opts = append(opts, pulse.PlaybackStereo)
	} else {
		opts = append(opts, pulse.PlaybackMono)
	}
	return opts
}

func applyRecordDevice(c *pulse.Client, device string, opts *[]pulse.RecordOption) error {
	if device == "" {
		return nil
	}
	if strings.HasSuffix(device, ".monitor") {
		sinkName := strings.TrimSuffix(device, ".monitor")
		sink, err := c.SinkByID(sinkName)
		if err != nil {
			return fmt.Errorf("monitor sink %q: %w", sinkName, err)
		}
		*opts = append(*opts, pulse.RecordMonitor(sink))
		return nil
	}
	src, err := c.SourceByID(device)
	if err != nil {
		return fmt.Errorf("source %q: %w", device, err)
	}
	*opts = append(*opts, pulse.RecordSource(src))
	return nil
}

func s16Format() byte {
	return proto.FormatInt16LE
}

type recordCloser struct {
	client *pulse.Client
	stream *pulse.RecordStream
	pw     *io.PipeWriter
	pr     *io.PipeReader
	once   sync.Once
}

func (r *recordCloser) Read(p []byte) (int, error) {
	if err := r.stream.Error(); err != nil {
		return 0, err
	}
	return r.pr.Read(p)
}

func (r *recordCloser) Close() error {
	r.stop()
	return nil
}

func (r *recordCloser) stop() {
	r.once.Do(func() {
		r.stream.Stop()
		r.stream.Close()
		r.pw.Close()
		r.pr.Close()
		r.client.Close()
	})
}

type playbackWriter struct {
	client    *pulse.Client
	stream    *pulse.PlaybackStream
	pr        *io.PipeReader
	pw        *io.PipeWriter
	once      sync.Once
	startOnce sync.Once
}

func (w *playbackWriter) Write(p []byte) (int, error) {
	if err := w.stream.Error(); err != nil {
		return 0, err
	}
	// Start the Pulse consumer before writing. The pipe is unbuffered, so
	// writing first then starting deadlocks: Write waits for a reader, and
	// Start never runs because it was scheduled after Write returns.
	w.startOnce.Do(func() {
		go w.stream.Start()
	})
	return w.pw.Write(p)
}

func (w *playbackWriter) Close() error {
	w.stop()
	return nil
}

func (w *playbackWriter) stop() {
	w.once.Do(func() {
		w.pw.Close()
		w.stream.Stop()
		w.stream.Close()
		w.pr.Close()
		w.client.Close()
	})
}
