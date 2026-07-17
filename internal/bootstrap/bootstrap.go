package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"

	"github.com/LDTorres/speech-to-text-sv/internal/app"
	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/log"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/control"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/notify"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/session"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/transcribe"
	"github.com/LDTorres/speech-to-text-sv/internal/platform"
	"go.uber.org/zap"
)

type Bootstrap struct {
	daemon   *app.Daemon
	logger   *zap.Logger
	platform config.ResolvedPlatform
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

	resolvedPlatform, err := cfg.ResolvePlatform(runtime.GOOS)
	if err != nil {
		return nil, err
	}

	logger, err := log.New(cfg.App.Environment)
	if err != nil {
		return nil, err
	}

	triggerWatcher := platform.NewTriggerWatcher(
		logger,
		cfg.Trigger,
		resolvedPlatform,
	)
	recorder := platform.NewRecorder(cfg.Audio, resolvedPlatform)
	transcriber := transcribe.NewWhisperRunner(
		cfg.Transcribe.BinaryPath,
		cfg.Transcribe.ModelPath,
		cfg.Transcribe.Language,
		cfg.Transcribe.Timeout,
	)
	clipboard := platform.NewClipboard(logger, cfg.Clipboard.EnablePaste, resolvedPlatform)

	var notifier notify.Notifier = notify.NewNoop()

	sessionService := session.NewService(logger, recorder, transcriber, clipboard, notifier)
	var controlServer *control.Server
	if resolvedPlatform.ExternalControl.Enabled {
		controlServer, err = control.NewServer(
			logger,
			resolvedPlatform.ExternalControl.SocketPath,
			sessionService,
		)
		if err != nil {
			return nil, fmt.Errorf("create external control server: %w", err)
		}
	}

	daemon := app.New(logger, triggerWatcher, controlServer, sessionService, cfg.App.ShutdownTimeout)

	return &Bootstrap{
		daemon:   daemon,
		logger:   logger,
		platform: resolvedPlatform,
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
