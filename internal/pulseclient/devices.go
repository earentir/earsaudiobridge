package pulseclient

import (
	"fmt"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

// Device is a sink or source with volume state.
type Device struct {
	Name   string
	Volume int
	Muted  bool
}

// ListNames returns sink or source names ("sinks" or "sources").
func ListNames(kind string) ([]string, error) {
	devs, err := Devices(stringsKind(kind))
	if err != nil {
		return nil, err
	}
	names := make([]string, len(devs))
	for i, d := range devs {
		names[i] = d.Name
	}
	return names, nil
}

func stringsKind(kind string) string {
	if kind == "sinks" {
		return "sink"
	}
	if kind == "sources" {
		return "source"
	}
	return kind
}

// Devices lists sinks or sources ("sink" or "source").
func Devices(kind string) ([]Device, error) {
	var out []Device
	err := withClient(appName, func(c *pulse.Client) error {
		switch kind {
		case "sink":
			var reply proto.GetSinkInfoListReply
			if err := c.RawRequest(&proto.GetSinkInfoList{}, &reply); err != nil {
				return err
			}
			out = make([]Device, len(reply))
			for i, info := range reply {
				out[i] = deviceFromInfo(info.SinkName, info.ChannelVolumes, info.Mute)
			}
		case "source":
			var reply proto.GetSourceInfoListReply
			if err := c.RawRequest(&proto.GetSourceInfoList{}, &reply); err != nil {
				return err
			}
			out = make([]Device, len(reply))
			for i, info := range reply {
				out[i] = deviceFromInfo(info.SourceName, info.ChannelVolumes, info.Mute)
			}
		default:
			return fmt.Errorf("pulseclient: unknown kind %q", kind)
		}
		return nil
	})
	return out, err
}

func deviceFromInfo(name string, vols proto.ChannelVolumes, muted bool) Device {
	percent := 0
	if len(vols) > 0 {
		percent = int(vols.Avg().Norm()*100 + 0.5)
	}
	return Device{Name: name, Volume: percent, Muted: muted}
}

// GetVolume returns volume percent for a sink or source.
func GetVolume(kind, name string) (int, error) {
	info, err := getDeviceInfo(kind, name)
	if err != nil {
		return 0, err
	}
	if len(info.vols) == 0 {
		return 0, fmt.Errorf("pulseclient: no volume for %s %s", kind, name)
	}
	return int(info.vols.Avg().Norm()*100 + 0.5), nil
}

// SetVolume sets volume percent (0–150) on a sink or source.
func SetVolume(kind, name string, percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 150 {
		percent = 150
	}
	vol := proto.NormVolume(float64(percent) / 100)
	return withClient(appName, func(c *pulse.Client) error {
		info, err := getDeviceInfo(kind, name)
		if err != nil {
			return err
		}
		cvol := make(proto.ChannelVolumes, len(info.vols))
		for i := range cvol {
			cvol[i] = vol
		}
		switch kind {
		case "sink":
			return c.RawRequest(&proto.SetSinkVolume{
				SinkIndex:      info.index,
				ChannelVolumes: cvol,
			}, nil)
		case "source":
			return c.RawRequest(&proto.SetSourceVolume{
				SourceIndex:    info.index,
				ChannelVolumes: cvol,
			}, nil)
		default:
			return fmt.Errorf("pulseclient: unknown kind %q", kind)
		}
	})
}

// GetMute returns mute state for a sink or source.
func GetMute(kind, name string) (bool, error) {
	info, err := getDeviceInfo(kind, name)
	if err != nil {
		return false, err
	}
	return info.muted, nil
}

// SetMute sets mute on a sink or source.
func SetMute(kind, name string, mute bool) error {
	return withClient(appName, func(c *pulse.Client) error {
		info, err := getDeviceInfo(kind, name)
		if err != nil {
			return err
		}
		switch kind {
		case "sink":
			return c.RawRequest(&proto.SetSinkMute{
				SinkIndex: info.index,
				Mute:      mute,
			}, nil)
		case "source":
			return c.RawRequest(&proto.SetSourceMute{
				SourceIndex: info.index,
				Mute:        mute,
			}, nil)
		default:
			return fmt.Errorf("pulseclient: unknown kind %q", kind)
		}
	})
}

type deviceInfo struct {
	index uint32
	vols  proto.ChannelVolumes
	muted bool
}

func getDeviceInfo(kind, name string) (deviceInfo, error) {
	var info deviceInfo
	err := withClient(appName, func(c *pulse.Client) error {
		switch kind {
		case "sink":
			s, err := lookupSink(c, name)
			if err != nil {
				return err
			}
			var reply proto.GetSinkInfoReply
			if err := c.RawRequest(&proto.GetSinkInfo{
				SinkIndex: s.SinkIndex(),
			}, &reply); err != nil {
				return err
			}
			info = deviceInfo{index: reply.SinkIndex, vols: reply.ChannelVolumes, muted: reply.Mute}
		case "source":
			s, err := lookupSource(c, name)
			if err != nil {
				return err
			}
			var reply proto.GetSourceInfoReply
			if err := c.RawRequest(&proto.GetSourceInfo{
				SourceIndex: s.SourceIndex(),
			}, &reply); err != nil {
				return err
			}
			info = deviceInfo{index: reply.SourceIndex, vols: reply.ChannelVolumes, muted: reply.Mute}
		default:
			return fmt.Errorf("pulseclient: unknown kind %q", kind)
		}
		return nil
	})
	return info, err
}

func lookupSink(c *pulse.Client, name string) (*pulse.Sink, error) {
	if name == "" || name == "@DEFAULT_SINK@" {
		return c.DefaultSink()
	}
	return c.SinkByID(name)
}

func lookupSource(c *pulse.Client, name string) (*pulse.Source, error) {
	if name == "" || name == "@DEFAULT_SOURCE@" {
		return c.DefaultSource()
	}
	return c.SourceByID(name)
}

// PlaybackDevices lists output sinks with a leading default entry.
func PlaybackDevices() ([]Device, error) {
	devs, err := Devices("sink")
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(devs)+1)
	out = append(out, Device{Name: ""})
	out = append(out, devs...)
	return out, nil
}

// CaptureDevices lists input sources with a leading default entry.
func CaptureDevices() ([]Device, error) {
	devs, err := Devices("source")
	if err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(devs)+1)
	out = append(out, Device{Name: ""})
	out = append(out, devs...)
	return out, nil
}
