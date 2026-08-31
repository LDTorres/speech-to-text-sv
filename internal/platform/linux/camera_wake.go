//go:build linux

package linux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	AudioWakeNone = "none"
	AudioWakeAuto = "auto"
)

var ErrCameraWakeUnavailable = errors.New("camera wake resolver unavailable")

type commandOutput func(context.Context, string, ...string) ([]byte, error)

type pipewireDumpObject struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info struct {
		Props map[string]json.RawMessage `json:"props"`
	} `json:"info"`
}

func resolveCameraVideoDevice(ctx context.Context, inputDevice string) (string, error) {
	return resolveCameraVideoDeviceWith(
		ctx,
		inputDevice,
		func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		"/sys",
		"/dev",
	)
}

func resolveCameraVideoDeviceWith(ctx context.Context, inputDevice string, output commandOutput, sysRoot, devRoot string) (string, error) {
	if inputDevice == "" {
		defaultSource, err := output(ctx, "pactl", "get-default-source")
		if err != nil {
			return "", fmt.Errorf("get default audio source: %w", err)
		}
		inputDevice = strings.TrimSpace(string(defaultSource))
	}

	dump, err := output(ctx, "pw-dump")
	if err != nil {
		return "", fmt.Errorf("dump PipeWire graph: %w", err)
	}

	var objects []pipewireDumpObject
	if err := json.Unmarshal(dump, &objects); err != nil {
		return "", fmt.Errorf("parse PipeWire graph: %w", err)
	}

	card, err := findAudioCard(objects, inputDevice)
	if err != nil && inputDevice != "" {
		// Older configurations may contain a generic value such as "pipewire"
		// rather than the actual source node. Resolve the current default before
		// deciding that the audio device is not a composite USB camera.
		if defaultSource, defaultErr := output(ctx, "pactl", "get-default-source"); defaultErr == nil {
			card, err = findAudioCard(objects, strings.TrimSpace(string(defaultSource)))
		}
	}
	if err != nil {
		return "", err
	}

	audioDevice, err := filepath.EvalSymlinks(filepath.Join(sysRoot, "class", "sound", "card"+card, "device"))
	if err != nil {
		return "", fmt.Errorf("resolve ALSA card %s: %w", card, err)
	}
	audioUSB := usbAncestor(audioDevice)
	if audioUSB == "" {
		return "", ErrCameraWakeUnavailable
	}

	videoLinks, err := filepath.Glob(filepath.Join(sysRoot, "class", "video4linux", "video*", "device"))
	if err != nil {
		return "", fmt.Errorf("find video devices: %w", err)
	}

	var fallback string
	for _, link := range videoLinks {
		videoDevice, err := filepath.EvalSymlinks(link)
		if err != nil || usbAncestor(videoDevice) != audioUSB {
			continue
		}

		name := filepath.Base(filepath.Dir(link))
		candidate := filepath.Join(devRoot, name)
		if stable, ok := stableVideoPath(candidate, devRoot); ok {
			if strings.Contains(filepath.Base(stable), "index0") {
				return stable, nil
			}
			fallback = stable
			continue
		}
		fallback = candidate
	}

	if fallback == "" {
		return "", ErrCameraWakeUnavailable
	}
	return fallback, nil
}

func findAudioCard(objects []pipewireDumpObject, inputDevice string) (string, error) {
	for _, object := range objects {
		if object.Type != "PipeWire:Interface:Node" {
			continue
		}
		if propString(object.Info.Props, "node.name") != inputDevice {
			continue
		}

		if card := firstPropString(object.Info.Props, "api.alsa.card", "alsa.card"); card != "" {
			return card, nil
		}
	}

	return "", fmt.Errorf("audio source %q is not an ALSA node", inputDevice)
}

func firstPropString(props map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		if value := propString(props, name); value != "" {
			return value
		}
	}
	return ""
}

func propString(props map[string]json.RawMessage, name string) string {
	raw, ok := props[name]
	if !ok {
		return ""
	}

	var stringValue string
	if json.Unmarshal(raw, &stringValue) == nil {
		return stringValue
	}

	var numberValue json.Number
	if json.Unmarshal(raw, &numberValue) == nil {
		return numberValue.String()
	}

	return ""
}

func usbAncestor(path string) string {
	for {
		if path == "/" || path == "." || path == "" {
			return ""
		}
		if _, err := os.Stat(filepath.Join(path, "idVendor")); err == nil {
			if _, err := os.Stat(filepath.Join(path, "idProduct")); err == nil {
				return path
			}
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

func stableVideoPath(candidate, devRoot string) (string, bool) {
	paths, err := filepath.Glob(filepath.Join(devRoot, "v4l", "by-id", "*"))
	if err != nil {
		return "", false
	}

	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}

	for _, path := range paths {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && resolved == resolvedCandidate {
			return path, true
		}
	}

	return "", false
}

func videoWakeArgs(videoDevice string) []string {
	return []string{
		"--no-config",
		"--really-quiet",
		"--vo=null",
		"--ao=null",
		"--demuxer-lavf-format=v4l2",
		"av://v4l2:" + videoDevice,
	}
}
