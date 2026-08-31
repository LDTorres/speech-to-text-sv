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

const maxWhisperOutputBytes = 1 << 20

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

type cappedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= b.Len() {
		b.truncated = true
		return len(p), nil
	}

	remaining := b.limit - b.Len()
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}

	return b.Buffer.Write(p)
}

func NewWhisperRunner(binaryPath, modelPath, language string, timeout time.Duration) *WhisperRunner {
	return &WhisperRunner{
		binaryPath: binaryPath,
		modelPath:  modelPath,
		language:   language,
		timeout:    timeout,
		execRunner: runWhisperCommand,
	}
}

func (r *WhisperRunner) Validate() error {
	return r.validate()
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
			"%w: run whisper.cpp: %w (stderr: %s)",
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

	var stdout cappedBuffer
	var stderr cappedBuffer
	stdout.limit = maxWhisperOutputBytes
	stderr.limit = maxWhisperOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.String(), stderr.String(), err
	}
	if stdout.truncated {
		return stdout.String(), stderr.String(), errors.New("whisper stdout exceeded the maximum size")
	}

	return stdout.String(), stderr.String(), nil
}

func validateBinaryPath(binaryPath string) error {
	if strings.Contains(binaryPath, string(filepath.Separator)) {
		info, err := os.Stat(binaryPath)
		if err != nil {
			return fmt.Errorf("%w: binary path %q: %w", ErrInvalidConfig, binaryPath, err)
		}

		if info.IsDir() {
			return fmt.Errorf("%w: binary path %q must be a file", ErrInvalidConfig, binaryPath)
		}
		if info.Mode()&0o111 == 0 {
			return fmt.Errorf("%w: binary path %q is not executable", ErrInvalidConfig, binaryPath)
		}

		return nil
	}

	if _, err := exec.LookPath(binaryPath); err != nil {
		return fmt.Errorf("%w: binary path %q: %w", ErrInvalidConfig, binaryPath, err)
	}

	return nil
}

func validateModelPath(modelPath string) error {
	info, err := os.Stat(modelPath)
	if err != nil {
		return fmt.Errorf("%w: model path %q: %w", ErrInvalidConfig, modelPath, err)
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
		if text == "" || !strings.HasPrefix(text, "[") {
			continue
		}

		end := strings.IndexByte(text, ']')
		if end < 0 || !isTimestampSegment(text[1:end]) {
			continue
		}
		text = strings.TrimSpace(text[end+1:])
		if text != "" {
			segments = append(segments, text)
		}
	}

	return strings.Join(segments, " ")
}

func isTimestampSegment(value string) bool {
	parts := strings.Split(value, "-->")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if _, err := time.Parse("15:04:05.000", strings.TrimSpace(part)); err != nil {
			return false
		}
	}
	return true
}
