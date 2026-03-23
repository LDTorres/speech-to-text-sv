package platform

import (
	"github.com/LDTorres/speech-to-text-sv/internal/modules/clipboard"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"go.uber.org/zap"
)

func NewTriggerWatcher(logger *zap.Logger) trigger.Watcher {
	return trigger.NewStubWatcher(logger)
}

func NewClipboard(logger *zap.Logger, enablePaste bool) clipboard.Clipboard {
	return clipboard.NewStub(logger, enablePaste)
}
