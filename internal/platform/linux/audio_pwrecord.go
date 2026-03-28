package linux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
)

var ErrPWRecordUnavailable = errors.New("pw-record executable not available")

type process interface {
	Start() error
	Wait() error
	Signal(os.Signal) error
	Kill() error
	Stderr() string
}

type processFactory func(ctx context.Context, name string, args []string) process

type PWRecordRecorder struct {
	tempDir     string
	fileName    string
	inputDevice string
	commandName string
	stopTimeout time.Duration
	newProcess  processFactory

	mu        sync.Mutex
	recording bool
	startedAt time.Time
	path      string
	process   process
}

func NewPWRecordRecorder(tempDir string, fileName string, inputDevice string) *PWRecordRecorder {
	return &PWRecordRecorder{
		tempDir:     tempDir,
		fileName:    fileName,
		inputDevice: inputDevice,
		commandName: "pw-record",
		stopTimeout: 2 * time.Second,
		newProcess: func(ctx context.Context, name string, args []string) process {
			return &execProcess{cmd: exec.CommandContext(ctx, name, args...)}
		},
	}
}

func (r *PWRecordRecorder) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mu.Lock()
	if r.recording {
		r.mu.Unlock()
		return audio.ErrAlreadyRecording
	}
	r.mu.Unlock()

	if _, err := exec.LookPath(r.commandName); err != nil {
		return fmt.Errorf("%w: %v", ErrPWRecordUnavailable, err)
	}

	outputPath := filepath.Join(r.tempDir, r.fileName)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create audio temp dir: %w", err)
	}

	proc := r.newProcess(ctx, r.commandName, r.buildArgs(outputPath))
	if err := proc.Start(); err != nil {
		return fmt.Errorf("start pw-record: %w", err)
	}

	r.mu.Lock()
	r.recording = true
	r.startedAt = time.Now().UTC()
	r.path = outputPath
	r.process = proc
	r.mu.Unlock()

	return nil
}

func (r *PWRecordRecorder) Stop(ctx context.Context) (audio.Recording, error) {
	select {
	case <-ctx.Done():
		return audio.Recording{}, ctx.Err()
	default:
	}

	r.mu.Lock()
	if !r.recording || r.process == nil {
		r.mu.Unlock()
		return audio.Recording{}, audio.ErrNotRecording
	}

	proc := r.process
	recording := audio.Recording{
		Path:      r.path,
		StartedAt: r.startedAt,
		StoppedAt: time.Now().UTC(),
	}
	r.recording = false
	r.startedAt = time.Time{}
	r.path = ""
	r.process = nil
	r.mu.Unlock()

	if err := proc.Signal(os.Interrupt); err != nil {
		if killErr := proc.Kill(); killErr != nil {
			return audio.Recording{}, fmt.Errorf("stop pw-record: signal interrupt: %w", err)
		}
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- proc.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil {
			if recordingLooksUsable(recording.Path) {
				return recording, nil
			}
			return audio.Recording{}, fmt.Errorf("stop pw-record: %w (stderr: %s)", err, proc.Stderr())
		}
	case <-time.After(r.stopTimeout):
		if err := proc.Kill(); err != nil {
			return audio.Recording{}, fmt.Errorf("stop pw-record: timed out waiting for process exit: %w", err)
		}
		err := <-waitCh
		if err != nil {
			return audio.Recording{}, fmt.Errorf("stop pw-record after kill: %w (stderr: %s)", err, proc.Stderr())
		}
	case <-ctx.Done():
		return audio.Recording{}, ctx.Err()
	}

	return recording, nil
}

func (r *PWRecordRecorder) buildArgs(outputPath string) []string {
	args := []string{
		"--rate", "16000",
		"--channels", "1",
	}
	if r.inputDevice != "" {
		args = append(args, "--target", r.inputDevice)
	}
	args = append(args, outputPath)
	return args
}

type execProcess struct {
	cmd    *exec.Cmd
	stderr bytes.Buffer
}

func recordingLooksUsable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir() && info.Size() > 0
}

func (p *execProcess) Start() error {
	p.cmd.Stderr = &p.stderr
	return p.cmd.Start()
}

func (p *execProcess) Wait() error {
	return p.cmd.Wait()
}

func (p *execProcess) Signal(sig os.Signal) error {
	if p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Signal(sig)
}

func (p *execProcess) Kill() error {
	if p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Kill()
}

func (p *execProcess) Stderr() string {
	return p.stderr.String()
}
