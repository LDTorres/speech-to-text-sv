package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/notify"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/transcribe"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSessionService_HandleTriggerPressed_StartsRecording(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	service := NewService(zap.NewNop(), recorder, &fakeTranscriber{}, &fakeClipboard{}, notify.NewNoop())

	err := service.HandleTriggerPressed(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, recorder.startCalls)
}

func TestSessionService_HandleTriggerReleased_WithoutActiveRecording_ReturnsError(t *testing.T) {
	t.Parallel()

	service := NewService(zap.NewNop(), &fakeRecorder{}, &fakeTranscriber{}, &fakeClipboard{}, notify.NewNoop())

	err := service.HandleTriggerReleased(context.Background())

	require.ErrorIs(t, err, ErrNotRecording)
}

func TestSessionService_HandleTriggerReleased_TranscribesAndPastes(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{
			Path:      "/tmp/sttd/last-recording.wav",
			StartedAt: time.Unix(10, 0),
			StoppedAt: time.Unix(11, 0),
		},
	}
	transcriber := fakeTranscriber{
		transcript: transcribe.Transcript{
			Text:     "hello deck",
			Duration: time.Second,
		},
	}
	clipboard := &fakeClipboard{}
	service := NewService(zap.NewNop(), recorder, &transcriber, clipboard, notify.NewNoop())

	require.NoError(t, service.HandleTriggerPressed(context.Background()))

	err := service.HandleTriggerReleased(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, recorder.startCalls)
	require.Equal(t, 1, recorder.stopCalls)
	require.Equal(t, 1, transcriber.calls)
	require.Equal(t, 1, clipboard.copyCalls)
	require.Equal(t, 1, clipboard.pasteCalls)
	require.Equal(t, "hello deck", clipboard.lastCopied)
}

func TestSessionService_HandleTriggerReleased_PasteFails_KeepsTranscriptForRetry(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	clipboard := &fakeClipboard{pasteErr: errors.New("paste device unavailable")}
	service := NewService(
		zap.NewNop(),
		recorder,
		&fakeTranscriber{transcript: transcribe.Transcript{Text: "retry me"}},
		clipboard,
		notify.NewNoop(),
	)

	require.NoError(t, service.HandleTriggerPressed(context.Background()))

	err := service.HandleTriggerReleased(context.Background())

	require.ErrorIs(t, err, ErrPasteFailed)

	clipboard.pasteErr = nil
	err = service.RetryLastPaste(context.Background())

	require.NoError(t, err)
	require.Equal(t, 2, clipboard.copyCalls)
	require.Equal(t, 2, clipboard.pasteCalls)
	require.Equal(t, "retry me", clipboard.lastCopied)
}

func TestSessionService_StartRecordingFailure_ReleasesState(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{startErr: errors.New("device unavailable")}
	service := NewService(zap.NewNop(), recorder, &fakeTranscriber{}, &fakeClipboard{}, notify.NewNoop())

	err := service.HandleTriggerPressed(context.Background())

	require.EqualError(t, err, "start recording: device unavailable")

	err = service.HandleTriggerPressed(context.Background())

	require.EqualError(t, err, "start recording: device unavailable")
	require.Equal(t, 2, recorder.startCalls)
}

func TestSessionService_TranscriptionFailure_DoesNotEnableRetry(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	service := NewService(
		zap.NewNop(),
		recorder,
		&fakeTranscriber{err: errors.New("whisper failed")},
		&fakeClipboard{},
		notify.NewNoop(),
	)

	require.NoError(t, service.HandleTriggerPressed(context.Background()))

	err := service.HandleTriggerReleased(context.Background())

	require.ErrorIs(t, err, ErrTranscriptionFailed)
	require.EqualError(t, service.RetryLastPaste(context.Background()), ErrNoTranscript.Error())
}

func TestSessionService_CopyFailure_DoesNotLoseTranscript(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	clipboard := &fakeClipboard{copyErr: errors.New("clipboard unavailable")}
	service := NewService(
		zap.NewNop(),
		recorder,
		&fakeTranscriber{transcript: transcribe.Transcript{Text: "copy me"}},
		clipboard,
		notify.NewNoop(),
	)

	require.NoError(t, service.HandleTriggerPressed(context.Background()))

	err := service.HandleTriggerReleased(context.Background())

	require.EqualError(t, err, "copy transcript: clipboard unavailable")

	clipboard.copyErr = nil
	err = service.RetryLastPaste(context.Background())

	require.NoError(t, err)
	require.Equal(t, "copy me", clipboard.lastCopied)
	require.Equal(t, 2, clipboard.copyCalls)
	require.Equal(t, 1, clipboard.pasteCalls)
}

func TestSessionService_DoubleTap_RetriesLastTranscript(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	clipboard := &fakeClipboard{pasteErr: errors.New("paste device unavailable")}
	service := NewService(
		zap.NewNop(),
		recorder,
		&fakeTranscriber{transcript: transcribe.Transcript{Text: "retry success"}},
		clipboard,
		notify.NewNoop(),
	)

	require.NoError(t, service.HandleTriggerPressed(context.Background()))
	require.ErrorIs(t, service.HandleTriggerReleased(context.Background()), ErrPasteFailed)

	clipboard.pasteErr = nil

	err := service.RetryLastPaste(context.Background())

	require.NoError(t, err)
	require.Equal(t, "retry success", clipboard.lastCopied)
}

func TestSessionService_DoubleTap_AfterSuccessfulRetry_RemainsConsistent(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	clipboard := &fakeClipboard{pasteErr: errors.New("paste device unavailable")}
	service := NewService(
		zap.NewNop(),
		recorder,
		&fakeTranscriber{transcript: transcribe.Transcript{Text: "keep retrying"}},
		clipboard,
		notify.NewNoop(),
	)

	require.NoError(t, service.HandleTriggerPressed(context.Background()))
	require.ErrorIs(t, service.HandleTriggerReleased(context.Background()), ErrPasteFailed)

	clipboard.pasteErr = nil
	require.NoError(t, service.RetryLastPaste(context.Background()))
	require.NoError(t, service.RetryLastPaste(context.Background()))
	require.Equal(t, "keep retrying", clipboard.lastCopied)
	require.Equal(t, 3, clipboard.copyCalls)
	require.Equal(t, 3, clipboard.pasteCalls)
}

func TestSessionService_RetryLastPaste_WithoutRetryEligible_ReturnsError(t *testing.T) {
	t.Parallel()

	service := NewService(zap.NewNop(), &fakeRecorder{}, &fakeTranscriber{}, &fakeClipboard{}, notify.NewNoop())
	service.clearTranscript()

	err := service.RetryLastPaste(context.Background())

	require.ErrorIs(t, err, ErrNoTranscript)
}

func TestSessionService_RetryLastPaste_NoTranscript_ReturnsError(t *testing.T) {
	t.Parallel()

	service := NewService(zap.NewNop(), &fakeRecorder{}, &fakeTranscriber{}, &fakeClipboard{}, notify.NewNoop())

	err := service.RetryLastPaste(context.Background())

	require.ErrorIs(t, err, ErrNoTranscript)
}

func TestSessionService_ConcurrentStartAndStop_ReturnsBusyWithoutReordering(t *testing.T) {
	t.Parallel()

	recorder := &blockingRecorder{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := NewService(zap.NewNop(), recorder, &fakeTranscriber{}, &fakeClipboard{}, notify.NewNoop())

	startDone := make(chan error, 1)
	go func() { startDone <- service.StartRecording(context.Background()) }()

	select {
	case <-recorder.started:
	case <-time.After(time.Second):
		t.Fatal("recorder start did not begin")
	}

	require.ErrorIs(t, service.StopRecordingAndProcess(context.Background()), ErrBusy)
	close(recorder.release)
	require.NoError(t, <-startDone)
	require.Equal(t, StateRecording, service.Status(context.Background()).State)
}

func TestSessionService_StartRecording_DuringProcessing_ReturnsBusy(t *testing.T) {
	t.Parallel()

	recorder := &fakeRecorder{
		stopRecording: audio.Recording{Path: "/tmp/sttd/last-recording.wav"},
	}
	transcriber := &blockingTranscriber{
		transcript: transcribe.Transcript{Text: "processing"},
		blocked:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	service := NewService(zap.NewNop(), recorder, transcriber, &fakeClipboard{}, notify.NewNoop())

	require.NoError(t, service.StartRecording(context.Background()))

	done := make(chan error, 1)
	go func() {
		done <- service.StopRecordingAndProcess(context.Background())
	}()

	select {
	case <-transcriber.blocked:
	case <-time.After(time.Second):
		t.Fatal("transcriber did not block")
	}

	require.ErrorIs(t, service.StartRecording(context.Background()), ErrBusy)

	close(transcriber.release)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("stop processing did not complete")
	}
}

type blockingRecorder struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingRecorder) Start(ctx context.Context) error {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *blockingRecorder) Stop(ctx context.Context) (audio.Recording, error) {
	return audio.Recording{}, audio.ErrNotRecording
}

type fakeRecorder struct {
	startErr      error
	stopErr       error
	stopRecording audio.Recording
	startCalls    int
	stopCalls     int
}

func (r *fakeRecorder) Start(ctx context.Context) error {
	r.startCalls++
	return r.startErr
}

func (r *fakeRecorder) Stop(ctx context.Context) (audio.Recording, error) {
	r.stopCalls++
	return r.stopRecording, r.stopErr
}

type fakeTranscriber struct {
	transcript transcribe.Transcript
	err        error
	calls      int
}

type blockingTranscriber struct {
	transcript transcribe.Transcript
	blocked    chan struct{}
	release    chan struct{}
	once       sync.Once
}

func (t *blockingTranscriber) TranscribeFile(ctx context.Context, path string) (transcribe.Transcript, error) {
	t.once.Do(func() {
		close(t.blocked)
	})

	select {
	case <-t.release:
		return t.transcript, nil
	case <-ctx.Done():
		return transcribe.Transcript{}, ctx.Err()
	}
}

func (t *fakeTranscriber) TranscribeFile(ctx context.Context, path string) (transcribe.Transcript, error) {
	t.calls++
	return t.transcript, t.err
}

type fakeClipboard struct {
	copyErr    error
	pasteErr   error
	lastCopied string
	copyCalls  int
	pasteCalls int
}

func (c *fakeClipboard) Copy(ctx context.Context, text string) error {
	c.copyCalls++
	c.lastCopied = text
	return c.copyErr
}

func (c *fakeClipboard) Paste(ctx context.Context) error {
	c.pasteCalls++
	return c.pasteErr
}
