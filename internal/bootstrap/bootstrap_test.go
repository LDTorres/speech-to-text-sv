package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootstrap_New_WiresRequiredDependencies(t *testing.T) {
	setProfileDefaultsForTest(t)
	if runtime.GOOS == "darwin" {
		t.Setenv("STTD_PLATFORM_PROFILE", "macos")
	} else {
		t.Setenv("STTD_PLATFORM_PROFILE", "linux")
	}
	setTranscriberPathsForTest(t)

	boot, err := New(context.Background())

	require.NoError(t, err)
	require.NotNil(t, boot)
	require.NotNil(t, boot.daemon)
	require.NotNil(t, boot.logger)
}

func TestBootstrap_New_UsesResolvedPlatformProfile(t *testing.T) {
	setProfileDefaultsForTest(t)
	if runtime.GOOS == "darwin" {
		t.Setenv("STTD_PLATFORM_PROFILE", "macos")
	} else {
		t.Setenv("STTD_PLATFORM_PROFILE", "linux")
	}
	setTranscriberPathsForTest(t)

	boot, err := New(context.Background())

	require.NoError(t, err)
	require.NotNil(t, boot)
	require.NotNil(t, boot.daemon)
	require.NotNil(t, boot.logger)
	require.NotEmpty(t, boot.platform.Profile)
	require.NotEmpty(t, boot.platform.TargetOS)
	require.NotEmpty(t, boot.platform.Trigger.Hotkey.Key)
	require.NotEmpty(t, boot.platform.Audio.Backend)
	require.NotEmpty(t, boot.platform.Clipboard.TargetOS)
}

func TestBootstrap_New_UsesMacOSResolvers(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macos bootstrap wiring only applies on darwin")
	}

	t.Setenv("STTD_PLATFORM_PROFILE", "macos")
	setProfileDefaultsForTest(t)
	setTranscriberPathsForTest(t)

	boot, err := New(context.Background())

	require.NoError(t, err)
	require.Equal(t, "darwin", boot.platform.TargetOS)
	require.Equal(t, "space", boot.platform.Trigger.Hotkey.Key)
	require.Equal(t, "macos_capture", boot.platform.Audio.Backend)
	require.Equal(t, "pbcopy", boot.platform.Clipboard.PreferredCopy)
	require.Equal(t, "osascript", boot.platform.Clipboard.PreferredPaste)
}

func TestBootstrap_New_UsesSteamDeckResolvers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("steam_deck bootstrap wiring only applies on linux")
	}

	t.Setenv("STTD_PLATFORM_PROFILE", "steam_deck")
	setProfileDefaultsForTest(t)
	setTranscriberPathsForTest(t)

	boot, err := New(context.Background())

	require.NoError(t, err)
	require.Equal(t, "linux", boot.platform.TargetOS)
	require.Equal(t, "f12", boot.platform.Trigger.Hotkey.Key)
	require.Equal(t, "toggle", boot.platform.Trigger.Mode)
	require.Equal(t, "pw-record", boot.platform.Audio.Backend)
}

func setTranscriberPathsForTest(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "whisper-cli")
	modelPath := filepath.Join(dir, "model.bin")
	require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	require.NoError(t, os.WriteFile(modelPath, []byte("model"), 0o600))
	t.Setenv("STTD_TRANSCRIBE_BINARY_PATH", binaryPath)
	t.Setenv("STTD_TRANSCRIBE_MODEL_PATH", modelPath)
}

func setProfileDefaultsForTest(t *testing.T) {
	t.Helper()
	// Keep repository-local .env files from changing profile defaults.
	t.Chdir(t.TempDir())
	for _, key := range []string{
		"STTD_TRIGGER_MODE",
		"STTD_TRIGGER_HOTKEY_MODIFIERS",
		"STTD_TRIGGER_HOTKEY_KEY",
		"STTD_PLATFORM_INTEGRATION",
		"STTD_EXTERNAL_CONTROL_ENABLED",
	} {
		previous, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, previous)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
}
