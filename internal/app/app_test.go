package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/session"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDaemon_Run_DispatchesPressEvent(t *testing.T) {
	t.Parallel()

	testRunDispatchesEvent(t, trigger.EventPress, func(session *fakeSessionService) int {
		return session.pressCalls
	})
}

func TestDaemon_Run_DispatchesReleaseEvent(t *testing.T) {
	t.Parallel()

	testRunDispatchesEvent(t, trigger.EventRelease, func(session *fakeSessionService) int {
		return session.releaseCalls
	})
}

func TestDaemon_Run_DispatchesDoubleTapEvent(t *testing.T) {
	t.Parallel()

	testRunDispatchesEvent(t, trigger.EventDoubleTap, func(session *fakeSessionService) int {
		return session.retryCalls
	})
}

func TestDaemon_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	watcher := newFakeWatcher()
	session := newFakeSessionService()
	daemon := New(zap.NewNop(), watcher, nil, session, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- daemon.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		return watcher.startCalls() == 1
	}, time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop after context cancellation")
	}

	require.Equal(t, 1, watcher.stopCalls())
	require.Zero(t, session.totalCalls())
}

func testRunDispatchesEvent(
	t *testing.T,
	eventKind trigger.EventKind,
	callCount func(session *fakeSessionService) int,
) {
	t.Helper()

	watcher := newFakeWatcher()
	session := newFakeSessionService()
	daemon := New(zap.NewNop(), watcher, nil, session, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- daemon.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		return watcher.startCalls() == 1
	}, time.Second, 10*time.Millisecond)

	watcher.emit(trigger.Event{
		Kind: eventKind,
		At:   time.Now().UTC(),
	})

	select {
	case <-session.called:
	case <-time.After(time.Second):
		t.Fatalf("daemon did not dispatch %s event", eventKind)
	}

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop after context cancellation")
	}

	require.Equal(t, 1, callCount(session))
	require.Equal(t, 1, watcher.stopCalls())
}

type fakeWatcher struct {
	events chan trigger.Event

	mu        sync.Mutex
	started   bool
	startCall int
	stopCall  int
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{
		events: make(chan trigger.Event, 1),
	}
}

func (w *fakeWatcher) Events() <-chan trigger.Event {
	return w.events
}

func (w *fakeWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.started = true
	w.startCall++

	return nil
}

func (w *fakeWatcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.started = false
	w.stopCall++

	return nil
}

func (w *fakeWatcher) emit(event trigger.Event) {
	w.events <- event
}

func (w *fakeWatcher) startCalls() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.startCall
}

func (w *fakeWatcher) stopCalls() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.stopCall
}

type fakeSessionService struct {
	called chan struct{}

	mu           sync.Mutex
	pressCalls   int
	releaseCalls int
	retryCalls   int
}

func newFakeSessionService() *fakeSessionService {
	return &fakeSessionService{
		called: make(chan struct{}, 3),
	}
}

func (s *fakeSessionService) StartRecording(ctx context.Context) error {
	s.mu.Lock()
	s.pressCalls++
	s.mu.Unlock()
	s.called <- struct{}{}
	return nil
}

func (s *fakeSessionService) StopRecordingAndProcess(ctx context.Context) error {
	s.mu.Lock()
	s.releaseCalls++
	s.mu.Unlock()
	s.called <- struct{}{}
	return nil
}

func (s *fakeSessionService) RetryLastPaste(ctx context.Context) error {
	s.mu.Lock()
	s.retryCalls++
	s.mu.Unlock()
	s.called <- struct{}{}
	return nil
}

func (s *fakeSessionService) ToggleRecording(ctx context.Context) error {
	return s.StartRecording(ctx)
}

func (s *fakeSessionService) Status(ctx context.Context) session.Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	return session.Status{
		State:          session.StateIdle,
		RetryAvailable: s.retryCalls > 0,
	}
}

func (s *fakeSessionService) totalCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.pressCalls + s.releaseCalls + s.retryCalls
}
