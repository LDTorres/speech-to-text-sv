//go:build darwin

package darwin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/stretchr/testify/require"
)

func TestMacRecorder_Start_OpensDeviceAndChangesState(t *testing.T) {
	t.Parallel()

	factory := &fakeCaptureFactory{
		session: &fakeCaptureSession{sampleRate: 16000, channels: 1},
	}
	recorder := NewRecorder(t.TempDir(), "recording.wav", "wav", "")
	recorder.factory = factory

	err := recorder.Start(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, factory.openCalls)
	require.True(t, factory.session.started)
}

func TestMacRecorder_Stop_WithoutStart_ReturnsError(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(t.TempDir(), "recording.wav", "wav", "")

	_, err := recorder.Stop(context.Background())

	require.ErrorIs(t, err, audio.ErrNotRecording)
}

func TestMacRecorder_Stop_AfterStart_WritesWAV(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	session := &fakeCaptureSession{sampleRate: 16000, channels: 1}
	factory := &fakeCaptureFactory{
		session: session,
		onOpen: func(onData func([]byte)) {
			onData([]byte{1, 2, 3, 4})
		},
	}
	recorder := NewRecorder(tempDir, "recording.wav", "wav", "")
	recorder.factory = factory

	require.NoError(t, recorder.Start(context.Background()))

	recording, err := recorder.Stop(context.Background())

	require.NoError(t, err)
	require.Equal(t, filepath.Join(tempDir, "recording.wav"), recording.Path)
	content, readErr := os.ReadFile(recording.Path)
	require.NoError(t, readErr)
	require.Len(t, content, 44+4)
}

func TestMacRecorder_Start_Twice_ReturnsError(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(t.TempDir(), "recording.wav", "wav", "")
	recorder.factory = &fakeCaptureFactory{
		session: &fakeCaptureSession{sampleRate: 16000, channels: 1},
	}

	require.NoError(t, recorder.Start(context.Background()))

	err := recorder.Start(context.Background())

	require.ErrorIs(t, err, audio.ErrAlreadyRecording)
}

type fakeCaptureFactory struct {
	session   *fakeCaptureSession
	err       error
	openCalls int
	onOpen    func(func([]byte))
}

func (f *fakeCaptureFactory) Open(inputDevice string, onData func([]byte)) (macCaptureSession, error) {
	f.openCalls++
	if f.onOpen != nil {
		f.onOpen(onData)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

type fakeCaptureSession struct {
	startErr   error
	stopErr    error
	closeErr   error
	sampleRate uint32
	channels   uint32
	started    bool
}

func (s *fakeCaptureSession) Start() error {
	if s.startErr != nil {
		return s.startErr
	}
	s.started = true
	return nil
}

func (s *fakeCaptureSession) Stop() error {
	if s.stopErr != nil {
		return s.stopErr
	}
	return nil
}

func (s *fakeCaptureSession) Close() error {
	if s.closeErr != nil {
		return s.closeErr
	}
	return nil
}

func (s *fakeCaptureSession) SampleRate() uint32 { return s.sampleRate }

func (s *fakeCaptureSession) Channels() uint32 { return s.channels }
