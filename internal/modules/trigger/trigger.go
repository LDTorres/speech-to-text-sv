package trigger

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

type EventKind string

const (
	EventPress     EventKind = "press"
	EventRelease   EventKind = "release"
	EventDoubleTap EventKind = "double_tap"
)

var (
	ErrWatcherAlreadyStarted = errors.New("trigger watcher already started")
	ErrWatcherNotStarted     = errors.New("trigger watcher not started")
)

type Event struct {
	Kind     EventKind
	At       time.Time
	Metadata map[string]string
}

type Watcher interface {
	Events() <-chan Event
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type StubWatcher struct {
	logger *zap.Logger

	mu     sync.Mutex
	events chan Event
	cancel context.CancelFunc
	done   chan struct{}
}

func NewStubWatcher(logger *zap.Logger) *StubWatcher {
	return &StubWatcher{
		logger: logger,
		events: make(chan Event),
	}
}

func (w *StubWatcher) Events() <-chan Event {
	return w.events
}

func (w *StubWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return ErrWatcherAlreadyStarted
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	w.cancel = cancel
	w.done = done

	go func() {
		defer close(done)
		<-runCtx.Done()
	}()

	w.logger.Info("trigger watcher started", zap.String("source", "stub"))

	return nil
}

func (w *StubWatcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	if cancel == nil {
		w.mu.Unlock()
		return ErrWatcherNotStarted
	}

	w.cancel = nil
	w.done = nil
	w.mu.Unlock()

	cancel()

	select {
	case <-done:
		w.logger.Info("trigger watcher stopped", zap.String("source", "stub"))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
