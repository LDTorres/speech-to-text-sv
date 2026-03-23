package platform

import (
	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/clipboard"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	darwinplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/darwin"
	linuxplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/linux"
	"go.uber.org/zap"
)

func NewTriggerWatcher(logger *zap.Logger, triggerCfg config.TriggerConfig, resolved config.ResolvedPlatform) trigger.Watcher {
	source := newTriggerSource(resolved)
	return trigger.NewWatcher(logger, source, resolved.Trigger.Source, triggerCfg.DoubleTapWindow)
}

func NewRecorder(cfg config.AudioConfig, resolved config.ResolvedPlatform) audio.Recorder {
	return newRecorder(cfg, resolved)
}

func NewClipboard(logger *zap.Logger, enablePaste bool, resolved config.ResolvedPlatform) clipboard.Clipboard {
	return clipboard.NewSystemForConfig(
		logger,
		enablePaste,
		resolved.Clipboard.TargetOS,
		resolved.Clipboard.PreferredCopy,
		resolved.Clipboard.PreferredPaste,
	)
}

func newTriggerSource(resolved config.ResolvedPlatform) trigger.Source {
	switch {
	case resolved.Trigger.Source == config.TriggerSourceHotkey:
		return darwinplatform.NewHotkeySource(resolved.Trigger.Hotkey)
	case resolved.Profile == config.PlatformProfileSteamDeck && resolved.Trigger.Source == config.TriggerSourceSteam:
		return linuxplatform.NewEvdevSource(
			resolved.Trigger.DevicePath,
			resolved.Trigger.EventType,
			resolved.Trigger.EventCode,
			resolved.Trigger.ActiveValue,
		)
	default:
		return trigger.NewStubSource()
	}
}

func newRecorder(cfg config.AudioConfig, resolved config.ResolvedPlatform) audio.Recorder {
	switch resolved.Audio.Backend {
	case config.AudioBackendMacOSCapture:
		return darwinplatform.NewRecorder(cfg.TempDir, cfg.FileName, cfg.SampleFormat, resolved.Audio.InputDevice)
	case config.AudioBackendPWRecord:
		return linuxplatform.NewPWRecordRecorder(cfg.TempDir, cfg.FileName, resolved.Audio.InputDevice)
	default:
		return audio.NewFileRecorder(cfg.TempDir, cfg.FileName, cfg.SampleFormat)
	}
}
