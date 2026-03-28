//go:build linux

package platform

import (
	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	linuxplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/linux"
)

func newTriggerSource(resolved config.ResolvedPlatform) trigger.Source {
	if resolved.Trigger.Source == config.TriggerSourceHotkey {
		return linuxplatform.NewHotkeySource(resolved.Trigger.Hotkey)
	}

	if resolved.Profile == config.PlatformProfileSteamDeck && resolved.Trigger.Source == config.TriggerSourceSteam {
		return linuxplatform.NewEvdevSource(
			resolved.Trigger.DevicePath,
			resolved.Trigger.EventType,
			resolved.Trigger.EventCode,
			resolved.Trigger.ActiveValue,
		)
	}

	return trigger.NewStubSource()
}

func newRecorder(cfg config.AudioConfig, resolved config.ResolvedPlatform) audio.Recorder {
	if resolved.Audio.Backend == config.AudioBackendPWRecord {
		return linuxplatform.NewPWRecordRecorder(cfg.TempDir, cfg.FileName, resolved.Audio.InputDevice)
	}

	return audio.NewFileRecorder(cfg.TempDir, cfg.FileName, cfg.SampleFormat)
}
