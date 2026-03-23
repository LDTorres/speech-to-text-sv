package linux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/stretchr/testify/require"
)

func TestPWRecordRecorder_Start_SpawnsProcess(t *testing.T) {
	t.Parallel()

	proc := &fakeProcess{}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		require.Equal(t, "pw-record", name)
		require.Equal(t, filepath.Join(recorder.tempDir, recorder.fileName), args[len(args)-1])
		return proc
	}
	recorder.commandName = testExecutableName(t)

	err := recorder.Start(context.Background())

	require.NoError(t, err)
	require.True(t, proc.started)
}

func TestPWRecordRecorder_Stop_WithoutStart_ReturnsError(t *testing.T) {
	t.Parallel()

	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")

	_, err := recorder.Stop(context.Background())

	require.ErrorIs(t, err, audio.ErrNotRecording)
}

func TestPWRecordRecorder_Stop_AfterStart_ReturnsRecording(t *testing.T) {
	t.Parallel()

	proc := &fakeProcess{}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	recorder.commandName = testExecutableName(t)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}

	require.NoError(t, recorder.Start(context.Background()))

	recording, err := recorder.Stop(context.Background())

	require.NoError(t, err)
	require.Equal(t, filepath.Join(recorder.tempDir, recorder.fileName), recording.Path)
	require.True(t, proc.interrupted)
}

func TestPWRecordRecorder_Start_Twice_ReturnsError(t *testing.T) {
	t.Parallel()

	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	recorder.commandName = testExecutableName(t)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return &fakeProcess{}
	}

	require.NoError(t, recorder.Start(context.Background()))

	err := recorder.Start(context.Background())

	require.ErrorIs(t, err, audio.ErrAlreadyRecording)
}

func TestPWRecordRecorder_ProcessFailure_ReturnsWrappedError(t *testing.T) {
	t.Parallel()

	proc := &fakeProcess{waitErr: errors.New("exit status 1")}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	recorder.commandName = testExecutableName(t)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}

	require.NoError(t, recorder.Start(context.Background()))

	_, err := recorder.Stop(context.Background())

	require.EqualError(t, err, "stop pw-record: exit status 1")
}

func TestPWRecordRecorder_Stop_UsesTimeout(t *testing.T) {
	t.Parallel()

	proc := &blockingProcess{}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	recorder.commandName = testExecutableName(t)
	recorder.stopTimeout = 10 * time.Millisecond
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}

	require.NoError(t, recorder.Start(context.Background()))

	_, err := recorder.Stop(context.Background())

	require.EqualError(t, err, "stop pw-record after kill: exit status 9")
	require.True(t, proc.killed)
}

type fakeProcess struct {
	started     bool
	interrupted bool
	waitErr     error
}

func (p *fakeProcess) Start() error {
	p.started = true
	return nil
}

func (p *fakeProcess) Wait() error {
	return p.waitErr
}

func (p *fakeProcess) Signal(sig os.Signal) error {
	p.interrupted = true
	return nil
}

func (p *fakeProcess) Kill() error {
	return nil
}

type blockingProcess struct {
	killed bool
	waitCh chan struct{}
}

func (p *blockingProcess) Start() error {
	if p.waitCh == nil {
		p.waitCh = make(chan struct{})
	}
	return nil
}

func (p *blockingProcess) Wait() error {
	if p.waitCh == nil {
		p.waitCh = make(chan struct{})
	}
	<-p.waitCh
	if p.killed {
		return errors.New("exit status 9")
	}
	return nil
}

func (p *blockingProcess) Signal(sig os.Signal) error {
	return nil
}

func (p *blockingProcess) Kill() error {
	p.killed = true
	close(p.waitCh)
	return nil
}

func testExecutableName(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "pw-record")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	originalPath := os.Getenv("PATH")
	require.NoError(t, os.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+originalPath))
	t.Cleanup(func() {
		require.NoError(t, os.Setenv("PATH", originalPath))
	})

	return "pw-record"
}
