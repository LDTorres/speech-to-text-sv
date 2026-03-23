package transcribe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var (
	ErrInvalidConfig = errors.New("invalid transcription configuration")
	ErrTranscription = errors.New("transcription failed")
)

type Transcript struct {
	Text     string
	Duration time.Duration
}

type Transcriber interface {
	TranscribeFile(ctx context.Context, path string) (Transcript, error)
}

type WhisperRunner struct {
	binaryPath string
	modelPath  string
	language   string
	timeout    time.Duration
}

func NewWhisperRunner(binaryPath string, modelPath string, language string, timeout time.Duration) *WhisperRunner {
	return &WhisperRunner{
		binaryPath: binaryPath,
		modelPath:  modelPath,
		language:   language,
		timeout:    timeout,
	}
}

func (r *WhisperRunner) TranscribeFile(ctx context.Context, path string) (Transcript, error) {
	if r.binaryPath == "" {
		return Transcript{}, fmt.Errorf("%w: binary path is required", ErrInvalidConfig)
	}

	if r.modelPath == "" {
		return Transcript{}, fmt.Errorf("%w: model path is required", ErrInvalidConfig)
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	startedAt := time.Now()
	cmd := exec.CommandContext(
		runCtx,
		r.binaryPath,
		"-m", r.modelPath,
		"-f", path,
		"-l", r.language,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Transcript{}, fmt.Errorf(
			"%w: run whisper.cpp: %v (stderr: %s)",
			ErrTranscription,
			err,
			strings.TrimSpace(stderr.String()),
		)
	}

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return Transcript{}, fmt.Errorf("%w: empty transcript output", ErrTranscription)
	}

	return Transcript{
		Text:     text,
		Duration: time.Since(startedAt),
	}, nil
}
