package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/notify"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/session"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/transcribe"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDaemon_Run_PressThenRelease_CompletesSessionFlow(t *testing.T) {
	t.Parallel()

	watcher := newIntegrationWatcher()
	clipboard := &integrationClipboard{}
	recorder := &integrationRecorder{
		recording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	transcriber := &integrationTranscriber{
		transcript: transcribe.Transcript{Text: "hello end to end", Duration: time.Second},
	}
	service := session.NewService(zap.NewNop(), recorder, transcriber, clipboard, notify.NewNoop())
	daemon := New(zap.NewNop(), watcher, service, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- daemon.Run(ctx)
	}()

	require.Eventually(t, func() bool { return watcher.startCalls() == 1 }, time.Second, 10*time.Millisecond)

	watcher.emit(trigger.Event{Kind: trigger.EventPress, At: time.Now().UTC()})
	watcher.emit(trigger.Event{Kind: trigger.EventRelease, At: time.Now().UTC()})

	require.Eventually(t, func() bool {
		return clipboard.pasteCalls() == 1
	}, time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop")
	}

	require.Equal(t, 1, recorder.startCalls())
	require.Equal(t, 1, recorder.stopCalls())
	require.Equal(t, 1, transcriber.calls())
	require.Equal(t, 1, clipboard.copyCalls())
	require.Equal(t, 1, clipboard.pasteCalls())
	require.Equal(t, "hello end to end", clipboard.lastCopied())
}

func TestDaemon_Run_DoubleTap_RetriesPaste(t *testing.T) {
	t.Parallel()

	watcher := newIntegrationWatcher()
	clipboard := &integrationClipboard{pasteErr: errors.New("paste failed")}
	recorder := &integrationRecorder{
		recording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	transcriber := &integrationTranscriber{
		transcript: transcribe.Transcript{Text: "retry end to end", Duration: time.Second},
	}
	service := session.NewService(zap.NewNop(), recorder, transcriber, clipboard, notify.NewNoop())
	daemon := New(zap.NewNop(), watcher, service, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- daemon.Run(ctx)
	}()

	require.Eventually(t, func() bool { return watcher.startCalls() == 1 }, time.Second, 10*time.Millisecond)

	watcher.emit(trigger.Event{Kind: trigger.EventPress, At: time.Now().UTC()})
	watcher.emit(trigger.Event{Kind: trigger.EventRelease, At: time.Now().UTC()})

	require.Eventually(t, func() bool {
		return clipboard.pasteCalls() == 1
	}, time.Second, 10*time.Millisecond)

	clipboard.setPasteErr(nil)
	watcher.emit(trigger.Event{Kind: trigger.EventDoubleTap, At: time.Now().UTC()})

	require.Eventually(t, func() bool {
		return clipboard.pasteCalls() == 2
	}, time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop")
	}

	require.Equal(t, "retry end to end", clipboard.lastCopied())
}

type integrationWatcher struct {
	events chan trigger.Event

	mu        sync.Mutex
	startCall int
	stopCall  int
}

func newIntegrationWatcher() *integrationWatcher {
	return &integrationWatcher{
		events: make(chan trigger.Event, 8),
	}
}

func (w *integrationWatcher) Events() <-chan trigger.Event {
	return w.events
}

func (w *integrationWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.startCall++
	return nil
}

func (w *integrationWatcher) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopCall++
	return nil
}

func (w *integrationWatcher) emit(event trigger.Event) {
	w.events <- event
}

func (w *integrationWatcher) startCalls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.startCall
}

type integrationRecorder struct {
	recording audio.Recording

	mu        sync.Mutex
	startCall int
	stopCall  int
}

func (r *integrationRecorder) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startCall++
	return nil
}

func (r *integrationRecorder) Stop(ctx context.Context) (audio.Recording, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopCall++
	return r.recording, nil
}

func (r *integrationRecorder) startCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startCall
}

func (r *integrationRecorder) stopCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopCall
}

type integrationTranscriber struct {
	transcript transcribe.Transcript

	mu        sync.Mutex
	callCount int
}

func (t *integrationTranscriber) TranscribeFile(ctx context.Context, path string) (transcribe.Transcript, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callCount++
	return t.transcript, nil
}

func (t *integrationTranscriber) calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.callCount
}

type integrationClipboard struct {
	mu         sync.Mutex
	copyCount  int
	pasteCount int
	copyErr    error
	pasteErr   error
	lastText   string
}

func (c *integrationClipboard) Copy(ctx context.Context, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.copyCount++
	c.lastText = text
	return c.copyErr
}

func (c *integrationClipboard) Paste(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pasteCount++
	return c.pasteErr
}

func (c *integrationClipboard) setPasteErr(err error) {
	c.mu.Lock()
	c.pasteErr = err
	c.mu.Unlock()
}

func (c *integrationClipboard) copyCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.copyCount
}

func (c *integrationClipboard) pasteCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pasteCount
}

func (c *integrationClipboard) lastCopied() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastText
}
