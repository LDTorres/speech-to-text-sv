//go:build darwin

package platform

import (
	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	darwinplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/darwin"
)

func newTriggerSource(resolved config.ResolvedPlatform) trigger.Source {
	if resolved.Trigger.Source == config.TriggerSourceHotkey {
		return darwinplatform.NewHotkeySource(resolved.Trigger.Hotkey)
	}

	return trigger.NewStubSource()
}

func newRecorder(cfg config.AudioConfig, resolved config.ResolvedPlatform) audio.Recorder {
	if resolved.Audio.Backend == config.AudioBackendMacOSCapture {
		return darwinplatform.NewRecorder(cfg.TempDir, cfg.FileName, cfg.SampleFormat, resolved.Audio.InputDevice)
	}

	return audio.NewFileRecorder(cfg.TempDir, cfg.FileName, cfg.SampleFormat)
}
