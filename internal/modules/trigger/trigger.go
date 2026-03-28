package trigger

import (
	"context"
	"errors"
	"fmt"
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

const (
	modeHold   = "hold"
	modeToggle = "toggle"
)

type SourceEventKind string

const (
	SourceEventPress   SourceEventKind = "press"
	SourceEventRelease SourceEventKind = "release"
)

var (
	ErrWatcherAlreadyStarted = errors.New("trigger watcher already started")
	ErrWatcherNotStarted     = errors.New("trigger watcher not started")
	ErrSourceAlreadyStarted  = errors.New("trigger source already started")
	ErrSourceNotStarted      = errors.New("trigger source not started")
)

type Event struct {
	Kind     EventKind
	At       time.Time
	Metadata map[string]string
}

type SourceEvent struct {
	Kind     SourceEventKind
	At       time.Time
	Metadata map[string]string
}

type Watcher interface {
	Events() <-chan Event
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Source interface {
	Events() <-chan SourceEvent
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type TriggerWatcher struct {
	logger          *zap.Logger
	source          Source
	sourceName      string
	mode            string
	doubleTapWindow time.Duration

	mu     sync.Mutex
	events chan Event
	cancel context.CancelFunc
	done   chan struct{}
}

func NewWatcher(logger *zap.Logger, source Source, sourceName string, mode string, doubleTapWindow time.Duration) *TriggerWatcher {
	return &TriggerWatcher{
		logger:          logger,
		source:          source,
		sourceName:      sourceName,
		mode:            mode,
		doubleTapWindow: doubleTapWindow,
		events:          make(chan Event, 8),
	}
}

func (w *TriggerWatcher) Events() <-chan Event {
	return w.events
}

func (w *TriggerWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		return ErrWatcherAlreadyStarted
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	if err := w.source.Start(runCtx); err != nil {
		cancel()
		return fmt.Errorf("start trigger source: %w", err)
	}

	w.cancel = cancel
	w.done = done

	go w.run(runCtx, done)

	w.logger.Info(
		"trigger watcher started",
		zap.String("source", w.sourceName),
		zap.String("mode", w.mode),
		zap.Duration("double_tap_window", w.doubleTapWindow),
	)

	return nil
}

func (w *TriggerWatcher) Stop(ctx context.Context) error {
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

	if err := w.source.Stop(ctx); err != nil && !errors.Is(err, ErrSourceNotStarted) {
		return fmt.Errorf("stop trigger source: %w", err)
	}

	select {
	case <-done:
		w.logger.Info("trigger watcher stopped", zap.String("source", w.sourceName))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *TriggerWatcher) run(ctx context.Context, done chan struct{}) {
	defer close(done)

	var lastPressAt time.Time
	var toggleActive bool

	for {
		select {
		case <-ctx.Done():
			return
		case rawEvent, ok := <-w.source.Events():
			if !ok {
				return
			}

			eventAt := rawEvent.At
			if eventAt.IsZero() {
				eventAt = time.Now().UTC()
			}

			switch rawEvent.Kind {
			case SourceEventPress:
				if w.mode == modeToggle {
					eventKind := EventPress
					if toggleActive {
						eventKind = EventRelease
					}

					toggleActive = !toggleActive
					if !w.emit(ctx, Event{
						Kind:     eventKind,
						At:       eventAt,
						Metadata: rawEvent.Metadata,
					}) {
						return
					}
					continue
				}

				if w.isDoubleTap(lastPressAt, eventAt) {
					lastPressAt = eventAt
					if !w.emit(ctx, Event{
						Kind:     EventDoubleTap,
						At:       eventAt,
						Metadata: rawEvent.Metadata,
					}) {
						return
					}
					continue
				}

				lastPressAt = eventAt
				if !w.emit(ctx, Event{
					Kind:     EventPress,
					At:       eventAt,
					Metadata: rawEvent.Metadata,
				}) {
					return
				}
			case SourceEventRelease:
				if w.mode == modeToggle {
					continue
				}

				if !w.emit(ctx, Event{
					Kind:     EventRelease,
					At:       eventAt,
					Metadata: rawEvent.Metadata,
				}) {
					return
				}
			}
		}
	}
}

func (w *TriggerWatcher) emit(ctx context.Context, event Event) bool {
	select {
	case w.events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (w *TriggerWatcher) isDoubleTap(lastPressAt time.Time, eventAt time.Time) bool {
	if lastPressAt.IsZero() || eventAt.Before(lastPressAt) {
		return false
	}

	return eventAt.Sub(lastPressAt) <= w.doubleTapWindow
}

type StubSource struct {
	mu     sync.Mutex
	events chan SourceEvent
	cancel context.CancelFunc
	done   chan struct{}
}

func NewStubSource() *StubSource {
	return &StubSource{
		events: make(chan SourceEvent),
	}
}

func (s *StubSource) Events() <-chan SourceEvent {
	return s.events
}

func (s *StubSource) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		return ErrSourceAlreadyStarted
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	s.cancel = cancel
	s.done = done

	go func() {
		defer close(done)
		<-runCtx.Done()
	}()

	return nil
}

func (s *StubSource) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	if cancel == nil {
		s.mu.Unlock()
		return ErrSourceNotStarted
	}

	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

	cancel()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
