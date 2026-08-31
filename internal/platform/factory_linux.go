//go:build linux

package platform

import (
	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	linuxplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/linux"
)

func newTriggerSource(resolved config.ResolvedPlatform) trigger.Source {
	return linuxplatform.NewHotkeySource(resolved.Trigger.Hotkey)
}

func newRecorder(cfg config.AudioConfig, resolved config.ResolvedPlatform) audio.Recorder {
	if resolved.Audio.Backend == config.AudioBackendPWRecord {
		return linuxplatform.NewPWRecordRecorderWithWake(
			cfg.TempDir,
			cfg.FileName,
			resolved.Audio.InputDevice,
			resolved.Audio.CameraWake,
			resolved.Audio.CameraVideoDevice,
		)
	}

	return audio.NewFileRecorder(cfg.TempDir, cfg.FileName, cfg.SampleFormat)
}
