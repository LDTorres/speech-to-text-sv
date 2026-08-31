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

type controlServer interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Daemon struct {
	logger          *zap.Logger
	triggerWatcher  trigger.Watcher
	controlServer   controlServer
	sessionService  session.Service
	shutdownTimeout time.Duration
}

func New(
	logger *zap.Logger,
	triggerWatcher trigger.Watcher,
	controlServer controlServer,
	sessionService session.Service,
	shutdownTimeout time.Duration,
) *Daemon {
	return &Daemon{
		logger:          logger,
		triggerWatcher:  triggerWatcher,
		controlServer:   controlServer,
		sessionService:  sessionService,
		shutdownTimeout: shutdownTimeout,
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	triggerStarted, triggerErr := d.startTriggerWatcher(ctx)
	controlStarted, controlErr := d.startControlServer(ctx)

	switch {
	case triggerStarted && triggerErr == nil:
	case controlStarted && controlErr == nil && triggerErr != nil:
		d.logger.Warn("trigger watcher unavailable; continuing with external control only", zap.Error(triggerErr))
	case triggerErr != nil && !controlStarted:
		return fmt.Errorf("start trigger watcher: %w", triggerErr)
	}

	switch {
	case controlStarted && controlErr == nil:
	case triggerStarted && controlErr != nil:
		d.logger.Warn("external control unavailable; continuing with trigger watcher only", zap.Error(controlErr))
	case controlErr != nil && !triggerStarted:
		return fmt.Errorf("start external control: %w", controlErr)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.shutdownTimeout)
		defer cancel()

		if d.sessionService.Status(shutdownCtx).State == session.StateRecording {
			if err := d.sessionService.StopRecordingAndProcess(shutdownCtx); err != nil {
				d.logger.Error("stop active recording during shutdown", zap.Error(err))
			}
		}

		if triggerStarted {
			if err := d.triggerWatcher.Stop(shutdownCtx); err != nil && !errors.Is(err, trigger.ErrWatcherNotStarted) {
				d.logger.Error("stop trigger watcher", zap.Error(err))
			}
		}

		if controlStarted {
			if err := d.controlServer.Stop(shutdownCtx); err != nil {
				d.logger.Error("stop external control", zap.Error(err))
			}
		}
	}()

	d.logger.Info(
		"daemon started",
		zap.Bool("trigger_watcher_started", triggerStarted),
		zap.Bool("external_control_started", controlStarted),
	)

	var triggerEvents <-chan trigger.Event
	if triggerStarted {
		triggerEvents = d.triggerWatcher.Events()
	}

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("daemon stopping")
			return nil
		case event, ok := <-triggerEvents:
			if !ok {
				if controlStarted {
					d.logger.Warn("trigger event stream closed; continuing with external control only")
					triggerEvents = nil
					continue
				}
				return errors.New("trigger event stream closed")
			}

			if err := d.handleEvent(ctx, event); err != nil {
				fields := []zap.Field{
					zap.String("event_kind", string(event.Kind)),
					zap.Time("event_at", event.At),
					zap.Error(err),
				}
				if source := event.Metadata["source"]; source != "" {
					fields = append(fields, zap.String("source", source))
				}
				d.logger.Error("handle trigger event", fields...)
			}
		}
	}
}

func (d *Daemon) handleEvent(ctx context.Context, event trigger.Event) error {
	switch event.Kind {
	case trigger.EventPress:
		return d.sessionService.StartRecording(ctx)
	case trigger.EventRelease:
		return d.sessionService.StopRecordingAndProcess(ctx)
	case trigger.EventDoubleTap:
		return d.sessionService.RetryLastPaste(ctx)
	default:
		return fmt.Errorf("unsupported trigger event kind: %s", event.Kind)
	}
}

func (d *Daemon) startTriggerWatcher(ctx context.Context) (bool, error) {
	if d.triggerWatcher == nil {
		return false, nil
	}

	if err := d.triggerWatcher.Start(ctx); err != nil {
		return false, err
	}

	return true, nil
}

func (d *Daemon) startControlServer(ctx context.Context) (bool, error) {
	if d.controlServer == nil {
		return false, nil
	}

	if err := d.controlServer.Start(ctx); err != nil {
		return false, err
	}

	return true, nil
}
