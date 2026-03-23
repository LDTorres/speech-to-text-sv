package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	"github.com/LDTorres/speech-to-text-sv/internal/app"
	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/log"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/notify"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/session"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/transcribe"
	"github.com/LDTorres/speech-to-text-sv/internal/platform"
	"go.uber.org/zap"
)

type Bootstrap struct {
	daemon *app.Daemon
	logger *zap.Logger
}

func New(ctx context.Context) (*Bootstrap, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger, err := log.New(cfg.App.Environment)
	if err != nil {
		return nil, err
	}

	triggerWatcher := platform.NewTriggerWatcher(logger)
	recorder := audio.NewStubRecorder(cfg.Audio.TempDir, cfg.Audio.FileName)
	transcriber := transcribe.NewWhisperRunner(
		cfg.Transcribe.BinaryPath,
		cfg.Transcribe.ModelPath,
		cfg.Transcribe.Language,
		cfg.Transcribe.Timeout,
	)
	clipboard := platform.NewClipboard(logger, cfg.Clipboard.EnablePaste)

	var notifier notify.Notifier = notify.NewNoop()

	sessionService := session.NewService(logger, recorder, transcriber, clipboard, notifier)
	daemon := app.New(logger, triggerWatcher, sessionService, cfg.App.ShutdownTimeout)

	return &Bootstrap{
		daemon: daemon,
		logger: logger,
	}, nil
}

func (b *Bootstrap) Run(ctx context.Context) error {
	return b.daemon.Run(ctx)
}

func (b *Bootstrap) Close() error {
	if b.logger == nil {
		return nil
	}

	if err := b.logger.Sync(); err != nil && !errors.Is(err, syscall.ENOTTY) {
		return fmt.Errorf("sync logger: %w", err)
	}

	return nil
}
