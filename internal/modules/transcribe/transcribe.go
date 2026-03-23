package transcribe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	execRunner execRunner
}

type execRunner func(ctx context.Context, binaryPath string, args []string) (string, string, error)

func NewWhisperRunner(binaryPath string, modelPath string, language string, timeout time.Duration) *WhisperRunner {
	return &WhisperRunner{
		binaryPath: binaryPath,
		modelPath:  modelPath,
		language:   language,
		timeout:    timeout,
		execRunner: runWhisperCommand,
	}
}

func (r *WhisperRunner) TranscribeFile(ctx context.Context, path string) (Transcript, error) {
	if err := r.validate(); err != nil {
		return Transcript{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	startedAt := time.Now()
	stdout, stderr, err := r.execRunner(runCtx, r.binaryPath, r.buildArgs(path))
	if err != nil {
		return Transcript{}, fmt.Errorf(
			"%w: run whisper.cpp: %v (stderr: %s)",
			ErrTranscription,
			err,
			strings.TrimSpace(stderr),
		)
	}

	text := parseTranscriptOutput(stdout)
	if text == "" {
		return Transcript{}, fmt.Errorf("%w: empty transcript output", ErrTranscription)
	}

	return Transcript{
		Text:     text,
		Duration: time.Since(startedAt),
	}, nil
}

func (r *WhisperRunner) buildArgs(path string) []string {
	args := []string{
		"-m", r.modelPath,
		"-f", path,
	}
	if strings.TrimSpace(r.language) != "" {
		args = append(args, "-l", strings.TrimSpace(r.language))
	}
	return args
}

func (r *WhisperRunner) validate() error {
	if r.binaryPath == "" {
		return fmt.Errorf("%w: binary path is required", ErrInvalidConfig)
	}

	if err := validateBinaryPath(r.binaryPath); err != nil {
		return err
	}

	if r.modelPath == "" {
		return fmt.Errorf("%w: model path is required", ErrInvalidConfig)
	}

	if err := validateModelPath(r.modelPath); err != nil {
		return err
	}

	if r.timeout <= 0 {
		return fmt.Errorf("%w: timeout must be greater than zero", ErrInvalidConfig)
	}

	return nil
}

func runWhisperCommand(ctx context.Context, binaryPath string, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, binaryPath, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.String(), stderr.String(), err
	}

	return stdout.String(), stderr.String(), nil
}

func validateBinaryPath(binaryPath string) error {
	if strings.Contains(binaryPath, string(filepath.Separator)) {
		info, err := os.Stat(binaryPath)
		if err != nil {
			return fmt.Errorf("%w: binary path %q: %v", ErrInvalidConfig, binaryPath, err)
		}

		if info.IsDir() {
			return fmt.Errorf("%w: binary path %q must be a file", ErrInvalidConfig, binaryPath)
		}

		return nil
	}

	if _, err := exec.LookPath(binaryPath); err != nil {
		return fmt.Errorf("%w: binary path %q: %v", ErrInvalidConfig, binaryPath, err)
	}

	return nil
}

func validateModelPath(modelPath string) error {
	info, err := os.Stat(modelPath)
	if err != nil {
		return fmt.Errorf("%w: model path %q: %v", ErrInvalidConfig, modelPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%w: model path %q must be a file", ErrInvalidConfig, modelPath)
	}

	return nil
}

func parseTranscriptOutput(output string) string {
	lines := strings.Split(output, "\n")
	segments := make([]string, 0, len(lines))

	for _, line := range lines {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}

		if strings.HasPrefix(text, "[") {
			if end := strings.Index(text, "]"); end >= 0 {
				text = strings.TrimSpace(text[end+1:])
			}
		}

		if text == "" {
			continue
		}

		segments = append(segments, text)
	}

	return strings.Join(segments, " ")
}
