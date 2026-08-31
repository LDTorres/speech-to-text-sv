//go:build linux

package linux

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
)

var (
	ErrPWRecordUnavailable = errors.New("pw-record executable not available")
	errProcessWaitTimeout  = errors.New("process wait timed out")
)

type process interface {
	Start() error
	Wait() error
	Signal(os.Signal) error
	Kill() error
	Stderr() string
}

type processFactory func(ctx context.Context, name string, args []string) process

type PWRecordRecorder struct {
	tempDir      string
	fileName     string
	inputDevice  string
	cameraWake   string
	videoDevice  string
	commandName  string
	wakeCommand  string
	stopTimeout  time.Duration
	newProcess   processFactory
	resolveVideo func(context.Context, string) (string, error)
	lookupPath   func(string) (string, error)
	wakeProcess  process

	operation sync.Mutex
	mu        sync.Mutex
	recording bool
	startedAt time.Time
	path      string
	process   process
}

func NewPWRecordRecorder(tempDir, fileName, inputDevice string) *PWRecordRecorder {
	return NewPWRecordRecorderWithWake(tempDir, fileName, inputDevice, AudioWakeNone, "")
}

func NewPWRecordRecorderWithWake(tempDir, fileName, inputDevice, cameraWake, videoDevice string) *PWRecordRecorder {
	return &PWRecordRecorder{
		tempDir:      tempDir,
		fileName:     fileName,
		inputDevice:  inputDevice,
		cameraWake:   cameraWake,
		videoDevice:  videoDevice,
		commandName:  "pw-record",
		wakeCommand:  "mpv",
		stopTimeout:  2 * time.Second,
		resolveVideo: resolveCameraVideoDevice,
		lookupPath:   exec.LookPath,
		newProcess: func(ctx context.Context, name string, args []string) process {
			return &execProcess{cmd: exec.CommandContext(ctx, name, args...)}
		},
	}
}

func (r *PWRecordRecorder) Start(ctx context.Context) error {
	r.operation.Lock()
	defer r.operation.Unlock()

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

	if _, err := r.lookupPath(r.commandName); err != nil {
		return fmt.Errorf("%w: %w", ErrPWRecordUnavailable, err)
	}

	outputPath := filepath.Join(r.tempDir, r.fileName)
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return fmt.Errorf("create audio temp dir: %w", err)
	}
	if err := os.Chmod(outputDir, 0o700); err != nil { // #nosec G302 -- 0700 is the private directory mode
		return fmt.Errorf("secure audio temp dir: %w", err)
	}
	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove previous audio recording: %w", err)
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

	if err := r.startCameraWake(ctx, outputPath); err != nil {
		r.mu.Lock()
		wakeProc := r.wakeProcess
		r.wakeProcess = nil
		r.recording = false
		r.startedAt = time.Time{}
		r.path = ""
		r.process = nil
		r.mu.Unlock()
		r.stopCameraWake(wakeProc)
		_ = proc.Signal(os.Interrupt)
		_ = proc.Kill()
		_ = waitProcess(proc, r.stopTimeout)
		return fmt.Errorf("start camera wake: %w", err)
	}

	return nil
}

func (r *PWRecordRecorder) Stop(ctx context.Context) (audio.Recording, error) {
	r.operation.Lock()
	defer r.operation.Unlock()

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
	wakeProc := r.wakeProcess
	r.wakeProcess = nil
	r.mu.Unlock()
	defer r.stopCameraWake(wakeProc)

	if err := proc.Signal(os.Interrupt); err != nil {
		if killErr := proc.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return recording, fmt.Errorf("stop pw-record: signal interrupt: %w", err)
		}
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- proc.Wait()
	}()

	timer := time.NewTimer(r.stopTimeout)
	defer timer.Stop()
	var waitErr error
	killed := false
	select {
	case waitErr = <-waitCh:
	case <-timer.C:
		killed = true
		if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return recording, fmt.Errorf("stop pw-record after timeout: %w", err)
		}
		cleanupTimer := time.NewTimer(r.stopTimeout)
		select {
		case waitErr = <-waitCh:
			cleanupTimer.Stop()
		case <-cleanupTimer.C:
			return recording, errors.New("stop pw-record: process did not exit after kill")
		}
	case <-ctx.Done():
		_ = proc.Kill()
		cleanupTimer := time.NewTimer(r.stopTimeout)
		select {
		case <-waitCh:
			cleanupTimer.Stop()
		case <-cleanupTimer.C:
			return recording, ctx.Err()
		}
		return recording, ctx.Err()
	}

	if err := os.Chmod(recording.Path, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		return recording, fmt.Errorf("secure audio recording: %w", err)
	}
	if waitErr != nil && !recordingLooksUsable(recording.Path) {
		message := "stop pw-record"
		if killed {
			message = "stop pw-record after kill"
		}
		return recording, fmt.Errorf("%s: %w (stderr: %s)", message, waitErr, proc.Stderr())
	}
	if !recordingLooksUsable(recording.Path) {
		return recording, errors.New("pw-record produced no audio frames")
	}

	return recording, nil
}

func (r *PWRecordRecorder) startCameraWake(ctx context.Context, outputPath string) error {
	if r.cameraWake == "" || r.cameraWake == AudioWakeNone {
		return nil
	}

	if r.cameraWake == AudioWakeAuto {
		ready, err := waitForAudioFrames(ctx, outputPath, 300*time.Millisecond)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
	}

	videoDevice := r.videoDevice
	if videoDevice == "" && r.cameraWake == AudioWakeAuto {
		resolved, err := r.resolveVideo(ctx, r.inputDevice)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, ErrCameraWakeUnavailable) {
				return nil
			}
			// Auto detection must not make ordinary microphones fail to start.
			return nil
		}
		videoDevice = resolved
	}
	if videoDevice == "" {
		return nil
	}
	if _, err := r.lookupPath(r.wakeCommand); err != nil {
		return fmt.Errorf("%s executable not available: %w", r.wakeCommand, err)
	}

	// Opening the video endpoint keeps composite webcam hardware awake while
	// pw-record consumes its sibling audio endpoint. Frames are discarded.
	proc := r.newProcess(ctx, r.wakeCommand, videoWakeArgs(videoDevice))
	if err := proc.Start(); err != nil {
		return fmt.Errorf("start %s: %w", r.wakeCommand, err)
	}

	r.mu.Lock()
	r.wakeProcess = proc
	r.mu.Unlock()

	// Give mpv a short head start so the camera firmware enables its audio
	// endpoint before the first user words are captured.
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitForAudioFrames(ctx context.Context, path string, timeout time.Duration) (bool, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if info, err := os.Stat(path); err == nil && info.Size() > 44 {
			return true, nil
		}

		select {
		case <-ticker.C:
		case <-timer.C:
			return false, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

func (r *PWRecordRecorder) stopCameraWake(proc process) {
	if proc == nil {
		return
	}

	if err := proc.Signal(os.Interrupt); err != nil {
		_ = proc.Kill()
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- proc.Wait() }()
	timer := time.NewTimer(r.stopTimeout)
	defer timer.Stop()
	select {
	case <-waitCh:
	case <-timer.C:
		_ = proc.Kill()
		cleanupTimer := time.NewTimer(r.stopTimeout)
		select {
		case <-waitCh:
			cleanupTimer.Stop()
		case <-cleanupTimer.C:
		}
	}
}

func waitProcess(proc process, timeout time.Duration) error {
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- proc.Wait()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-waitCh:
		return err
	case <-timer.C:
		return errProcessWaitTimeout
	}
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
	file, err := os.Open(path) // #nosec G304 -- the recorder validates and owns its configured output path
	if err != nil {
		return false
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil || info.IsDir() || info.Size() <= 44 {
		return false
	}

	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil || string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return false
	}

	offset := int64(12)
	for offset+8 <= info.Size() {
		chunkHeader := make([]byte, 8)
		if _, err := io.ReadFull(file, chunkHeader); err != nil {
			return false
		}
		offset += 8

		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		if chunkSize > info.Size()-offset {
			return false
		}
		if string(chunkHeader[0:4]) == "data" {
			return chunkSize > 0
		}

		skip := chunkSize + chunkSize%2
		if skip > info.Size()-offset {
			return false
		}
		if _, err := file.Seek(skip, io.SeekCurrent); err != nil {
			return false
		}
		offset += skip
	}

	return false
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
