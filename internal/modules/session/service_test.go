package session

import (
	"context"
	"errors"
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
