package platform

import (
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/clipboard"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"go.uber.org/zap"
)

func NewTriggerWatcher(logger *zap.Logger, triggerCfg config.TriggerConfig, resolved config.ResolvedPlatform) trigger.Watcher {
	source := newTriggerSource(resolved)
	return trigger.NewWatcher(logger, source, "hotkey", resolved.Trigger.Mode, triggerCfg.DoubleTapWindow)
}

func NewRecorder(cfg config.AudioConfig, resolved config.ResolvedPlatform) audio.Recorder {
	return newRecorder(cfg, resolved)
}

func NewClipboard(logger *zap.Logger, enablePaste bool, timeout time.Duration, resolved config.ResolvedPlatform) clipboard.Clipboard {
	return clipboard.NewSystemForConfig(
		logger,
		enablePaste,
		timeout,
		resolved.Clipboard.TargetOS,
		resolved.Clipboard.PreferredCopy,
		resolved.Clipboard.PreferredPaste,
	)
}
