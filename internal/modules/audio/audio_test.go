package audio

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecorder_Start_SetsRecordingState(t *testing.T) {
	t.Parallel()

	recorder := NewFileRecorder(t.TempDir(), "last-recording.wav", "wav")

	err := recorder.Start(context.Background())

	require.NoError(t, err)

	recorder.mu.Lock()
	recording := recorder.recording
	startedAt := recorder.startedAt
	recorder.mu.Unlock()

	require.True(t, recording)
	require.False(t, startedAt.IsZero())
}

func TestRecorder_Stop_WithoutStart_ReturnsError(t *testing.T) {
	t.Parallel()

	recorder := NewFileRecorder(t.TempDir(), "last-recording.wav", "wav")

	_, err := recorder.Stop(context.Background())

	require.ErrorIs(t, err, ErrNotRecording)
}

func TestRecorder_Stop_AfterStart_ReturnsRecordingWithPath(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	recorder := NewFileRecorder(tempDir, "last-recording.wav", "wav")

	require.NoError(t, recorder.Start(context.Background()))

	recording, err := recorder.Stop(context.Background())

	require.NoError(t, err)
	require.Equal(t, filepath.Join(tempDir, "last-recording.wav"), recording.Path)
	require.False(t, recording.StartedAt.IsZero())
	require.False(t, recording.StoppedAt.IsZero())
	require.True(t, recording.StoppedAt.Equal(recording.StartedAt) || recording.StoppedAt.After(recording.StartedAt))

	content, readErr := os.ReadFile(recording.Path)
	require.NoError(t, readErr)
	require.NotEmpty(t, content)
	info, statErr := os.Stat(recording.Path)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	recorder.mu.Lock()
	isRecording := recorder.recording
	startedAt := recorder.startedAt
	recorder.mu.Unlock()

	require.False(t, isRecording)
	require.True(t, startedAt.IsZero())
}

func TestRecorder_Start_Twice_ReturnsError(t *testing.T) {
	t.Parallel()

	recorder := NewFileRecorder(t.TempDir(), "last-recording.wav", "wav")

	require.NoError(t, recorder.Start(context.Background()))

	err := recorder.Start(context.Background())

	require.ErrorIs(t, err, ErrAlreadyRecording)
}
