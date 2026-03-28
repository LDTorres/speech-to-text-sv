package trigger

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTriggerWatcher_EmitsPress(t *testing.T) {
	t.Parallel()

	watcher, source := newTestWatcher(modeHold, 400*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, watcher.Start(ctx))
	defer func() {
		require.NoError(t, watcher.Stop(context.Background()))
	}()

	at := time.Unix(100, 0).UTC()
	source.emit(SourceEvent{Kind: SourceEventPress, At: at})

	event := readEvent(t, watcher.Events())
	require.Equal(t, EventPress, event.Kind)
	require.Equal(t, at, event.At)
}

func TestTriggerWatcher_EmitsRelease(t *testing.T) {
	t.Parallel()

	watcher, source := newTestWatcher(modeHold, 400*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, watcher.Start(ctx))
	defer func() {
		require.NoError(t, watcher.Stop(context.Background()))
	}()

	at := time.Unix(101, 0).UTC()
	source.emit(SourceEvent{Kind: SourceEventRelease, At: at})

	event := readEvent(t, watcher.Events())
	require.Equal(t, EventRelease, event.Kind)
	require.Equal(t, at, event.At)
}

func TestTriggerWatcher_TwoQuickPresses_EmitDoubleTap(t *testing.T) {
	t.Parallel()

	watcher, source := newTestWatcher(modeHold, 400*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, watcher.Start(ctx))
	defer func() {
		require.NoError(t, watcher.Stop(context.Background()))
	}()

	firstAt := time.Unix(200, 0).UTC()
	secondAt := firstAt.Add(200 * time.Millisecond)

	source.emit(SourceEvent{Kind: SourceEventPress, At: firstAt})
	source.emit(SourceEvent{Kind: SourceEventPress, At: secondAt})

	firstEvent := readEvent(t, watcher.Events())
	secondEvent := readEvent(t, watcher.Events())

	require.Equal(t, EventPress, firstEvent.Kind)
	require.Equal(t, EventDoubleTap, secondEvent.Kind)
	require.Equal(t, secondAt, secondEvent.At)
}

func TestTriggerWatcher_PressesOutsideWindow_DoNotEmitDoubleTap(t *testing.T) {
	t.Parallel()

	watcher, source := newTestWatcher(modeHold, 400*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, watcher.Start(ctx))
	defer func() {
		require.NoError(t, watcher.Stop(context.Background()))
	}()

	firstAt := time.Unix(300, 0).UTC()
	secondAt := firstAt.Add(500 * time.Millisecond)

	source.emit(SourceEvent{Kind: SourceEventPress, At: firstAt})
	source.emit(SourceEvent{Kind: SourceEventPress, At: secondAt})

	firstEvent := readEvent(t, watcher.Events())
	secondEvent := readEvent(t, watcher.Events())

	require.Equal(t, EventPress, firstEvent.Kind)
	require.Equal(t, EventPress, secondEvent.Kind)
	require.Equal(t, secondAt, secondEvent.At)
}

func TestTriggerWatcher_Stop_UnblocksRunLoop(t *testing.T) {
	t.Parallel()

	watcher, source := newTestWatcher(modeHold, 400*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, watcher.Start(ctx))

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- watcher.Stop(context.Background())
	}()

	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("watcher stop did not complete")
	}

	require.Equal(t, 1, source.startCalls())
	require.Equal(t, 1, source.stopCalls())
}

func TestTriggerWatcher_ToggleMode_PressesAlternateBetweenStartAndStop(t *testing.T) {
	t.Parallel()

	watcher, source := newTestWatcher(modeToggle, 400*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, watcher.Start(ctx))
	defer func() {
		require.NoError(t, watcher.Stop(context.Background()))
	}()

	firstAt := time.Unix(400, 0).UTC()
	secondAt := firstAt.Add(100 * time.Millisecond)

	source.emit(SourceEvent{Kind: SourceEventPress, At: firstAt})
	source.emit(SourceEvent{Kind: SourceEventPress, At: secondAt})

	firstEvent := readEvent(t, watcher.Events())
	secondEvent := readEvent(t, watcher.Events())

	require.Equal(t, EventPress, firstEvent.Kind)
	require.Equal(t, EventRelease, secondEvent.Kind)
	require.Equal(t, secondAt, secondEvent.At)
}

func TestTriggerWatcher_ToggleMode_IgnoresSourceRelease(t *testing.T) {
	t.Parallel()

	watcher, source := newTestWatcher(modeToggle, 400*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, watcher.Start(ctx))
	defer func() {
		require.NoError(t, watcher.Stop(context.Background()))
	}()

	source.emit(SourceEvent{Kind: SourceEventRelease, At: time.Unix(500, 0).UTC()})

	select {
	case event := <-watcher.Events():
		t.Fatalf("unexpected trigger event: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

func newTestWatcher(mode string, doubleTapWindow time.Duration) (*TriggerWatcher, *fakeSource) {
	source := newFakeSource()
	watcher := NewWatcher(zap.NewNop(), source, "fake", mode, doubleTapWindow)
	return watcher, source
}

func readEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("expected trigger event")
		return Event{}
	}
}

type fakeSource struct {
	events chan SourceEvent

	mu        sync.Mutex
	startCall int
	stopCall  int
}

func newFakeSource() *fakeSource {
	return &fakeSource{
		events: make(chan SourceEvent, 8),
	}
}

func (s *fakeSource) Events() <-chan SourceEvent {
	return s.events
}

func (s *fakeSource) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.startCall++
	return nil
}

func (s *fakeSource) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopCall++
	return nil
}

func (s *fakeSource) emit(event SourceEvent) {
	s.events <- event
}

func (s *fakeSource) startCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.startCall
}

func (s *fakeSource) stopCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stopCall
}
