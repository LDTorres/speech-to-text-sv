package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigLoad_ReadsDotEnvFromWorkingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	writeDotEnv(t, tempDir, "STTD_TRANSCRIBE_BINARY_PATH=/tmp/from-dotenv\nSTTD_TRANSCRIBE_MODEL_PATH=/tmp/model-from-dotenv.bin\n")
	withWorkingDir(t, tempDir)
	withUnsetEnv(t, "STTD_TRANSCRIBE_BINARY_PATH")
	withUnsetEnv(t, "STTD_TRANSCRIBE_MODEL_PATH")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "/tmp/from-dotenv", cfg.Transcribe.BinaryPath)
	require.Equal(t, "/tmp/model-from-dotenv.bin", cfg.Transcribe.ModelPath)
}

func TestConfigLoad_DoesNotMutateProcessEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	writeDotEnv(t, tempDir, "STTD_APP_ENV=test-from-dotenv\n")
	withWorkingDir(t, tempDir)
	withUnsetEnv(t, "STTD_APP_ENV")

	_, err := Load()
	require.NoError(t, err)

	_, exists := os.LookupEnv("STTD_APP_ENV")
	require.False(t, exists)
}

func TestConfigLoad_ProcessEnvironmentOverridesDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	writeDotEnv(t, tempDir, "STTD_TRANSCRIBE_BINARY_PATH=/tmp/from-dotenv\n")
	withWorkingDir(t, tempDir)
	t.Setenv("STTD_TRANSCRIBE_BINARY_PATH", "/tmp/from-process-env")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "/tmp/from-process-env", cfg.Transcribe.BinaryPath)
}

func writeDotEnv(t *testing.T, dir, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644))
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	currentDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))

	t.Cleanup(func() {
		require.NoError(t, os.Chdir(currentDir))
	})
}

func withUnsetEnv(t *testing.T, key string) {
	t.Helper()

	previousValue, existed := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))

	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.Setenv(key, previousValue))
			return
		}

		require.NoError(t, os.Unsetenv(key))
	})
}
