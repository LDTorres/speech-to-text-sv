//go:build linux

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
	configureTestCommand(recorder)

	err := recorder.Start(context.Background())

	require.NoError(t, err)
	require.True(t, proc.started)
}

func TestPWRecordRecorder_Start_RemovesPreviousRecording(t *testing.T) {
	t.Parallel()

	proc := &fakeProcess{}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	configureTestCommand(recorder)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}
	outputPath := filepath.Join(recorder.tempDir, recorder.fileName)
	require.NoError(t, os.WriteFile(outputPath, usableWAV(), 0o644))

	require.NoError(t, recorder.Start(context.Background()))
	require.True(t, proc.started)
	_, err := os.Stat(outputPath)
	require.ErrorIs(t, err, os.ErrNotExist)
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
	configureTestCommand(recorder)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}

	require.NoError(t, recorder.Start(context.Background()))
	require.NoError(t, os.WriteFile(filepath.Join(recorder.tempDir, recorder.fileName), usableWAV(), 0o644))

	recording, err := recorder.Stop(context.Background())

	require.NoError(t, err)
	require.Equal(t, filepath.Join(recorder.tempDir, recorder.fileName), recording.Path)
	require.True(t, proc.interrupted)
}

func TestPWRecordRecorder_Start_Twice_ReturnsError(t *testing.T) {
	t.Parallel()

	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	configureTestCommand(recorder)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return &fakeProcess{}
	}

	require.NoError(t, recorder.Start(context.Background()))

	err := recorder.Start(context.Background())

	require.ErrorIs(t, err, audio.ErrAlreadyRecording)
}

func TestPWRecordRecorder_ProcessFailure_ReturnsWrappedError(t *testing.T) {
	t.Parallel()

	proc := &fakeProcess{waitErr: errors.New("exit status 1"), stderr: "device failure"}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	configureTestCommand(recorder)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}

	require.NoError(t, recorder.Start(context.Background()))

	_, err := recorder.Stop(context.Background())

	require.EqualError(t, err, "stop pw-record: exit status 1 (stderr: device failure)")
}

func TestPWRecordRecorder_Stop_AllowsUsableRecordingWhenProcessExitsNonZero(t *testing.T) {
	t.Parallel()

	proc := &fakeProcess{waitErr: errors.New("exit status 1")}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	configureTestCommand(recorder)
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}

	require.NoError(t, recorder.Start(context.Background()))
	require.NoError(t, os.WriteFile(filepath.Join(recorder.tempDir, recorder.fileName), usableWAV(), 0o644))

	recording, err := recorder.Stop(context.Background())

	require.NoError(t, err)
	require.Equal(t, filepath.Join(recorder.tempDir, recorder.fileName), recording.Path)
}

func TestRecordingLooksUsable_RejectsHeaderOnlyWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.wav")
	require.NoError(t, os.WriteFile(path, []byte{
		'R', 'I', 'F', 'F', 36, 0, 0, 0, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0,
		0x80, 0x3e, 0, 0, 0, 0x7d, 0, 0, 2, 0, 16, 0,
		'd', 'a', 't', 'a', 0, 0, 0, 0,
	}, 0o644))

	require.False(t, recordingLooksUsable(path))
}

func TestRecordingLooksUsable_AcceptsAudioFrames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.wav")
	require.NoError(t, os.WriteFile(path, usableWAV(), 0o644))

	require.True(t, recordingLooksUsable(path))
}

func usableWAV() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 40, 0, 0, 0, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0,
		0x80, 0x3e, 0, 0, 0, 0x7d, 0, 0, 2, 0, 16, 0,
		'd', 'a', 't', 'a', 2, 0, 0, 0, 0, 0,
	}
}

func TestPWRecordRecorder_Stop_UsesTimeout(t *testing.T) {
	t.Parallel()

	proc := &blockingProcess{}
	recorder := NewPWRecordRecorder(t.TempDir(), "recording.wav", "")
	configureTestCommand(recorder)
	recorder.stopTimeout = 10 * time.Millisecond
	recorder.newProcess = func(ctx context.Context, name string, args []string) process {
		return proc
	}

	require.NoError(t, recorder.Start(context.Background()))

	_, err := recorder.Stop(context.Background())

	require.EqualError(t, err, "stop pw-record after kill: exit status 9 (stderr: )")
	require.True(t, proc.killed)
}

type fakeProcess struct {
	started     bool
	interrupted bool
	waitErr     error
	stderr      string
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

func (p *fakeProcess) Stderr() string {
	return p.stderr
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

func (p *blockingProcess) Stderr() string {
	return ""
}

func configureTestCommand(recorder *PWRecordRecorder) {
	recorder.commandName = "pw-record"
	recorder.lookupPath = func(string) (string, error) {
		return "/usr/bin/pw-record", nil
	}
}
