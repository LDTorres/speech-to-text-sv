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

	mu        sync.Mutex
	recording bool
	startedAt time.Time
}

func NewFileRecorder(tempDir string, fileName string, sampleFormat string) *FileRecorder {
	return &FileRecorder{
		tempDir:      tempDir,
		fileName:     fileName,
		sampleFormat: strings.ToLower(strings.TrimSpace(sampleFormat)),
	}
}

func (r *FileRecorder) Start(ctx context.Context) error {
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

	if err := os.MkdirAll(r.tempDir, 0o755); err != nil {
		return Recording{}, fmt.Errorf("create audio temp dir: %w", err)
	}

	if err := os.WriteFile(recording.Path, r.placeholderData(), 0o644); err != nil {
		return Recording{}, fmt.Errorf("write audio recording: %w", err)
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
