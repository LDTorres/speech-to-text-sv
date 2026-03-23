package audio

import (
	"context"
	"errors"
	"path/filepath"
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

type StubRecorder struct {
	tempDir  string
	fileName string

	mu        sync.Mutex
	recording bool
	startedAt time.Time
}

func NewStubRecorder(tempDir string, fileName string) *StubRecorder {
	return &StubRecorder{
		tempDir:  tempDir,
		fileName: fileName,
	}
}

func (r *StubRecorder) Start(ctx context.Context) error {
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

func (r *StubRecorder) Stop(ctx context.Context) (Recording, error) {
	select {
	case <-ctx.Done():
		return Recording{}, ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.recording {
		return Recording{}, ErrNotRecording
	}

	recording := Recording{
		Path:      filepath.Join(r.tempDir, r.fileName),
		StartedAt: r.startedAt,
		StoppedAt: time.Now().UTC(),
	}

	r.recording = false
	r.startedAt = time.Time{}

	return recording, nil
}
