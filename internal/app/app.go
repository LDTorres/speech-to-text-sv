package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/session"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"go.uber.org/zap"
)

type Daemon struct {
	logger          *zap.Logger
	triggerWatcher  trigger.Watcher
	sessionService  session.Service
	shutdownTimeout time.Duration
}

func New(
	logger *zap.Logger,
	triggerWatcher trigger.Watcher,
	sessionService session.Service,
	shutdownTimeout time.Duration,
) *Daemon {
	return &Daemon{
		logger:          logger,
		triggerWatcher:  triggerWatcher,
		sessionService:  sessionService,
		shutdownTimeout: shutdownTimeout,
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := d.triggerWatcher.Start(ctx); err != nil {
		return fmt.Errorf("start trigger watcher: %w", err)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), d.shutdownTimeout)
		defer cancel()

		if err := d.triggerWatcher.Stop(shutdownCtx); err != nil && !errors.Is(err, trigger.ErrWatcherNotStarted) {
			d.logger.Error("stop trigger watcher", zap.Error(err))
		}
	}()

	d.logger.Info("daemon started")

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("daemon stopping")
			return nil
		case event, ok := <-d.triggerWatcher.Events():
			if !ok {
				return errors.New("trigger event stream closed")
			}

			if err := d.handleEvent(ctx, event); err != nil {
				d.logger.Error(
					"handle trigger event",
					zap.String("event_kind", string(event.Kind)),
					zap.Time("event_at", event.At),
					zap.Error(err),
				)
			}
		}
	}
}

func (d *Daemon) handleEvent(ctx context.Context, event trigger.Event) error {
	switch event.Kind {
	case trigger.EventPress:
		return d.sessionService.HandleTriggerPressed(ctx)
	case trigger.EventRelease:
		return d.sessionService.HandleTriggerReleased(ctx)
	case trigger.EventDoubleTap:
		return d.sessionService.RetryLastPaste(ctx)
	default:
		return fmt.Errorf("unsupported trigger event kind: %s", event.Kind)
	}
}
