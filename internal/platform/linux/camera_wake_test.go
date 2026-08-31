//go:build linux

package linux

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveCameraVideoDevice_MatchesAudioAndVideoOnSameUSBDevice(t *testing.T) {
	sysRoot := t.TempDir()
	devRoot := t.TempDir()
	usbRoot := filepath.Join(sysRoot, "devices", "usb1", "1-1")
	audioInterface := filepath.Join(usbRoot, "1-1:1.2")
	videoInterface := filepath.Join(usbRoot, "1-1:1.0")
	require.NoError(t, os.MkdirAll(audioInterface, 0o755))
	require.NoError(t, os.MkdirAll(videoInterface, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(usbRoot, "idVendor"), []byte("3564"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(usbRoot, "idProduct"), []byte("fef9"), 0o644))

	soundLink := filepath.Join(sysRoot, "class", "sound", "card2", "device")
	videoLink := filepath.Join(sysRoot, "class", "video4linux", "video0", "device")
	require.NoError(t, os.MkdirAll(filepath.Dir(soundLink), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(videoLink), 0o755))
	require.NoError(t, os.Symlink(audioInterface, soundLink))
	require.NoError(t, os.Symlink(videoInterface, videoLink))

	videoNode := filepath.Join(devRoot, "video0")
	stableVideoNode := filepath.Join(devRoot, "v4l", "by-id", "usb-obsbot-video-index0")
	require.NoError(t, os.MkdirAll(filepath.Dir(stableVideoNode), 0o755))
	require.NoError(t, os.WriteFile(videoNode, nil, 0o644))
	require.NoError(t, os.Symlink(videoNode, stableVideoNode))

	sourceName := "alsa_input.usb-obsbot.analog-stereo"
	dump := []pipewireDumpObject{{
		ID:   94,
		Type: "PipeWire:Interface:Node",
	}}
	dump[0].Info.Props = map[string]json.RawMessage{
		"node.name":     json.RawMessage(`"` + sourceName + `"`),
		"api.alsa.card": json.RawMessage(`"2"`),
	}
	dumpBytes, err := json.Marshal(dump)
	require.NoError(t, err)

	resolved, err := resolveCameraVideoDeviceWith(
		context.Background(),
		sourceName,
		func(context.Context, string, ...string) ([]byte, error) {
			return dumpBytes, nil
		},
		sysRoot,
		devRoot,
	)

	require.NoError(t, err)
	require.Equal(t, stableVideoNode, resolved)
}

func TestResolveCameraVideoDevice_DoesNotMatchUnrelatedVideoDevice(t *testing.T) {
	sysRoot := t.TempDir()
	devRoot := t.TempDir()
	audioUSB := filepath.Join(sysRoot, "devices", "usb1", "1-1")
	videoUSB := filepath.Join(sysRoot, "devices", "usb2", "2-1")
	for _, path := range []string{audioUSB, videoUSB} {
		require.NoError(t, os.MkdirAll(filepath.Join(path, "interface"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(path, "idVendor"), []byte("3564"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(path, "idProduct"), []byte("fef9"), 0o644))
	}

	soundLink := filepath.Join(sysRoot, "class", "sound", "card2", "device")
	videoLink := filepath.Join(sysRoot, "class", "video4linux", "video0", "device")
	require.NoError(t, os.MkdirAll(filepath.Dir(soundLink), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(videoLink), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(audioUSB, "interface"), soundLink))
	require.NoError(t, os.Symlink(filepath.Join(videoUSB, "interface"), videoLink))
	require.NoError(t, os.WriteFile(filepath.Join(devRoot, "video0"), nil, 0o644))

	dump := []byte(`[{"id":94,"type":"PipeWire:Interface:Node","info":{"props":{"node.name":"alsa_input.usb-mic.analog-stereo","api.alsa.card":"2"}}}]`)
	_, err := resolveCameraVideoDeviceWith(
		context.Background(),
		"alsa_input.usb-mic.analog-stereo",
		func(context.Context, string, ...string) ([]byte, error) {
			return dump, nil
		},
		sysRoot,
		devRoot,
	)

	require.ErrorIs(t, err, ErrCameraWakeUnavailable)
	require.NotErrorIs(t, err, context.Canceled)
}

func TestVideoWakeArgs_DiscardVideoOutput(t *testing.T) {
	require.Equal(t, []string{
		"--no-config",
		"--really-quiet",
		"--vo=null",
		"--ao=null",
		"--demuxer-lavf-format=v4l2",
		"av://v4l2:/dev/video0",
	}, videoWakeArgs("/dev/video0"))
}
