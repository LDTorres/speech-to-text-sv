package audio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrAlreadyRecording = errors.New("audio recording already active")
	ErrNotRecording     = errors.New("audio recording not active")
)

type Recording struct {
	Path      string
	StartedAt time.Time
	StoppedAt time.Time
}

type Recorder interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) (Recording, error)
}

type FileRecorder struct {
	tempDir      string
	fileName     string
	sampleFormat string

	operation sync.Mutex
	mu        sync.Mutex
	recording bool
	startedAt time.Time
}

func NewFileRecorder(tempDir, fileName, sampleFormat string) *FileRecorder {
	return &FileRecorder{
		tempDir:      tempDir,
		fileName:     fileName,
		sampleFormat: strings.ToLower(strings.TrimSpace(sampleFormat)),
	}
}

func (r *FileRecorder) Start(ctx context.Context) error {
	r.operation.Lock()
	defer r.operation.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recording {
		return ErrAlreadyRecording
	}

	r.recording = true
	r.startedAt = time.Now().UTC()

	return nil
}

func (r *FileRecorder) Stop(ctx context.Context) (Recording, error) {
	r.operation.Lock()
	defer r.operation.Unlock()

	select {
	case <-ctx.Done():
		return Recording{}, ctx.Err()
	default:
	}

	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return Recording{}, ErrNotRecording
	}

	recording := Recording{
		Path:      filepath.Join(r.tempDir, r.fileName),
		StartedAt: r.startedAt,
		StoppedAt: time.Now().UTC(),
	}

	r.recording = false
	r.startedAt = time.Time{}
	r.mu.Unlock()

	if err := os.MkdirAll(r.tempDir, 0o700); err != nil {
		return Recording{}, fmt.Errorf("create audio temp dir: %w", err)
	}
	// #nosec G302 -- 0700 is the private directory mode
	if err := os.Chmod(r.tempDir, 0o700); err != nil {
		return Recording{}, fmt.Errorf("secure audio temp dir: %w", err)
	}

	file, err := os.OpenFile(recording.Path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return Recording{}, fmt.Errorf("write audio recording: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return Recording{}, fmt.Errorf("secure audio recording: %w", err)
	}
	if _, err := file.Write(r.placeholderData()); err != nil {
		_ = file.Close()
		return Recording{}, fmt.Errorf("write audio recording: %w", err)
	}
	if err := file.Close(); err != nil {
		return Recording{}, fmt.Errorf("close audio recording: %w", err)
	}

	return recording, nil
}

func (r *FileRecorder) placeholderData() []byte {
	if r.sampleFormat == "wav" || r.sampleFormat == "" {
		return zeroLengthWAV()
	}

	return []byte{}
}

func zeroLengthWAV() []byte {
	return []byte{
		'R', 'I', 'F', 'F',
		36, 0, 0, 0,
		'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ',
		16, 0, 0, 0,
		1, 0,
		1, 0,
		0x80, 0x3e, 0x00, 0x00,
		0x00, 0x7d, 0x00, 0x00,
		2, 0,
		16, 0,
		'd', 'a', 't', 'a',
		0, 0, 0, 0,
	}
}
