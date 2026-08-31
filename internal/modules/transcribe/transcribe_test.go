package transcribe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWhisperRunner_MissingBinaryPath_ReturnsInvalidConfig(t *testing.T) {
	t.Parallel()

	runner := NewWhisperRunner("", "/tmp/model.bin", "en", time.Second)

	_, err := runner.TranscribeFile(context.Background(), "/tmp/audio.wav")

	require.ErrorIs(t, err, ErrInvalidConfig)
	require.EqualError(t, err, "invalid transcription configuration: binary path is required")
}

func TestWhisperRunner_MissingModelPath_ReturnsInvalidConfig(t *testing.T) {
	t.Parallel()

	binaryPath := writeExecutableScript(t, "#!/bin/sh\necho ignored\n")
	runner := NewWhisperRunner(binaryPath, "", "en", time.Second)

	_, err := runner.TranscribeFile(context.Background(), "/tmp/audio.wav")

	require.ErrorIs(t, err, ErrInvalidConfig)
	require.EqualError(t, err, "invalid transcription configuration: model path is required")
}

func TestWhisperRunner_ProcessFailure_ReturnsWrappedError(t *testing.T) {
	t.Parallel()

	binaryPath := writeExecutableScript(t, "#!/bin/sh\necho whisper failed >&2\nexit 1\n")
	modelPath := writeModelFile(t)
	runner := NewWhisperRunner(binaryPath, modelPath, "en", time.Second)
	runner.execRunner = func(ctx context.Context, binaryPath string, args []string) (string, string, error) {
		return "", "whisper failed", errors.New("exit status 1")
	}

	_, err := runner.TranscribeFile(context.Background(), "/tmp/audio.wav")

	require.ErrorIs(t, err, ErrTranscription)
	require.ErrorContains(t, err, "whisper failed")
}

func TestWhisperRunner_Validate_RejectsNonExecutableBinary(t *testing.T) {
	t.Parallel()

	binaryPath := filepath.Join(t.TempDir(), "whisper-cli")
	require.NoError(t, os.WriteFile(binaryPath, []byte("binary"), 0o600))
	runner := NewWhisperRunner(binaryPath, writeModelFile(t), "en", time.Second)

	require.ErrorIs(t, runner.Validate(), ErrInvalidConfig)
}

func TestWhisperRunner_EmptyStdout_ReturnsTranscriptionError(t *testing.T) {
	t.Parallel()

	binaryPath := writeExecutableScript(t, "#!/bin/sh\nprintf '   '\n")
	modelPath := writeModelFile(t)
	runner := NewWhisperRunner(binaryPath, modelPath, "en", time.Second)
	runner.execRunner = func(ctx context.Context, binaryPath string, args []string) (string, string, error) {
		return "   ", "", nil
	}

	_, err := runner.TranscribeFile(context.Background(), "/tmp/audio.wav")

	require.ErrorIs(t, err, ErrTranscription)
	require.EqualError(t, err, "transcription failed: empty transcript output")
}

func TestParseTranscriptOutput_IgnoresNonSegmentLines(t *testing.T) {
	t.Parallel()

	output := "warning: device unavailable\n[WARN] not a transcript\n[00:00:00.000 --> 00:00:01.000] keep this\n"

	require.Equal(t, "keep this", parseTranscriptOutput(output))
}

func TestWhisperRunner_Success_ParsesTranscript(t *testing.T) {
	t.Parallel()

	binaryPath := writeExecutableScript(
		t,
		"#!/bin/sh\nprintf '[00:00:00.000 --> 00:00:00.500] hello\\n[00:00:00.500 --> 00:00:01.000] world\\n'\n",
	)
	modelPath := writeModelFile(t)
	runner := NewWhisperRunner(binaryPath, modelPath, "en", time.Second)
	runner.execRunner = func(ctx context.Context, binaryPath string, args []string) (string, string, error) {
		return "[00:00:00.000 --> 00:00:00.500] hello\n[00:00:00.500 --> 00:00:01.000] world\n", "", nil
	}

	transcript, err := runner.TranscribeFile(context.Background(), "/tmp/audio.wav")

	require.NoError(t, err)
	require.Equal(t, "hello world", transcript.Text)
	require.Positive(t, transcript.Duration)
}

func TestWhisperRunner_BuildArgs_OmitsLanguageWhenEmpty(t *testing.T) {
	t.Parallel()

	runner := NewWhisperRunner("/tmp/whisper-cli", "/tmp/model.bin", "", time.Second)

	args := runner.buildArgs("/tmp/audio.wav")

	require.Equal(t, []string{"-m", "/tmp/model.bin", "-f", "/tmp/audio.wav"}, args)
}

func TestWhisperRunner_BuildArgs_IncludesLanguageWhenConfigured(t *testing.T) {
	t.Parallel()

	runner := NewWhisperRunner("/tmp/whisper-cli", "/tmp/model.bin", "es", time.Second)

	args := runner.buildArgs("/tmp/audio.wav")

	require.Equal(t, []string{"-m", "/tmp/model.bin", "-f", "/tmp/audio.wav", "-l", "es"}, args)
}

func writeExecutableScript(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "whisper-cli")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
	return path
}

func writeModelFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "model.bin")
	require.NoError(t, os.WriteFile(path, []byte("model"), 0o644))
	return path
}
