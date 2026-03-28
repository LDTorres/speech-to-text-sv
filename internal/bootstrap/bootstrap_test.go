package bootstrap

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootstrap_New_WiresRequiredDependencies(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Setenv("STTD_PLATFORM_PROFILE", "macos")
	} else {
		t.Setenv("STTD_PLATFORM_PROFILE", "linux")
	}
	t.Setenv("STTD_TRANSCRIBE_BINARY_PATH", "")
	t.Setenv("STTD_TRANSCRIBE_MODEL_PATH", "")

	boot, err := New(context.Background())

	require.NoError(t, err)
	require.NotNil(t, boot)
	require.NotNil(t, boot.daemon)
	require.NotNil(t, boot.logger)
}

func TestBootstrap_New_UsesResolvedPlatformProfile(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Setenv("STTD_PLATFORM_PROFILE", "macos")
	} else {
		t.Setenv("STTD_PLATFORM_PROFILE", "linux")
	}
	t.Setenv("STTD_TRANSCRIBE_BINARY_PATH", "")
	t.Setenv("STTD_TRANSCRIBE_MODEL_PATH", "")

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
	t.Setenv("STTD_TRANSCRIBE_BINARY_PATH", "")
	t.Setenv("STTD_TRANSCRIBE_MODEL_PATH", "")

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
	t.Setenv("STTD_TRANSCRIBE_BINARY_PATH", "")
	t.Setenv("STTD_TRANSCRIBE_MODEL_PATH", "")

	boot, err := New(context.Background())

	require.NoError(t, err)
	require.Equal(t, "linux", boot.platform.TargetOS)
	require.Equal(t, "f12", boot.platform.Trigger.Hotkey.Key)
	require.Equal(t, "toggle", boot.platform.Trigger.Mode)
	require.Equal(t, "pw-record", boot.platform.Audio.Backend)
}
