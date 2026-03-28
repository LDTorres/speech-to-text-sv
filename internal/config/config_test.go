package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigLoad_DefaultPlatformProfile_Linux(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, PlatformProfileLinux, cfg.Platform.Profile)
}

func TestConfigLoad_DefaultTriggerConfig(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, TriggerModeHold, cfg.Trigger.Mode)
	require.Equal(t, "cmd+shift", cfg.Trigger.Hotkey.Modifiers)
	require.Equal(t, "space", cfg.Trigger.Hotkey.Key)
}

func TestConfigLoad_InvalidTimeout_ReturnsError(t *testing.T) {
	t.Setenv("STTD_APP_SHUTDOWN_TIMEOUT", "0s")

	_, err := Load()

	require.EqualError(t, err, "invalid configuration: shutdown timeout must be greater than zero")
}

func TestConfigLoad_InvalidPlatformProfile_ReturnsError(t *testing.T) {
	t.Setenv("STTD_PLATFORM_PROFILE", "wrong")

	_, err := Load()

	require.EqualError(t, err, `invalid configuration: unsupported platform profile "wrong"`)
}

func TestConfigLoad_InvalidMacHotkey_ReturnsError(t *testing.T) {
	t.Setenv("STTD_TRIGGER_HOTKEY_MODIFIERS", "cmd+weird")

	_, err := Load()

	require.EqualError(t, err, `invalid configuration: unsupported trigger hotkey modifier "weird"`)
}

func TestConfigLoad_InvalidTriggerMode_ReturnsError(t *testing.T) {
	t.Setenv("STTD_TRIGGER_MODE", "weird")

	_, err := Load()

	require.EqualError(t, err, `invalid configuration: unsupported trigger mode "weird"`)
}

func TestConfigLoad_EmptyHotkeyModifiers_AreAllowed(t *testing.T) {
	t.Setenv("STTD_TRIGGER_HOTKEY_MODIFIERS", "")
	t.Setenv("STTD_TRIGGER_HOTKEY_KEY", "f13")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "", cfg.Trigger.Hotkey.Modifiers)
	require.Equal(t, "f13", cfg.Trigger.Hotkey.Key)
}

func TestResolvePlatform_MacOS_UsesExpectedDefaults(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileMacOS},
	}.ResolvePlatform("darwin")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileMacOS, resolved.Profile)
	require.Equal(t, "darwin", resolved.TargetOS)
	require.Equal(t, TriggerModeHold, resolved.Trigger.Mode)
	require.Equal(t, []string{"cmd", "shift"}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "space", resolved.Trigger.Hotkey.Key)
	require.Equal(t, AudioBackendMacOSCapture, resolved.Audio.Backend)
	require.Equal(t, "darwin", resolved.Clipboard.TargetOS)
	require.Equal(t, "pbcopy", resolved.Clipboard.PreferredCopy)
	require.Equal(t, PasteMethodOSAScript, resolved.Clipboard.PreferredPaste)
}

func TestResolvePlatform_Linux_UsesRealDesktopDefaults(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileLinux},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileLinux, resolved.Profile)
	require.Equal(t, "linux", resolved.TargetOS)
	require.Equal(t, TriggerModeHold, resolved.Trigger.Mode)
	require.Equal(t, []string{"ctrl", "shift"}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "space", resolved.Trigger.Hotkey.Key)
	require.Equal(t, AudioBackendPWRecord, resolved.Audio.Backend)
	require.Equal(t, "linux", resolved.Clipboard.TargetOS)
	require.Equal(t, "wl-copy", resolved.Clipboard.PreferredCopy)
	require.Equal(t, PasteMethodWType, resolved.Clipboard.PreferredPaste)
}

func TestResolvePlatform_SteamDeck_UsesHotkeyToggleDefaults(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileSteamDeck},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileSteamDeck, resolved.Profile)
	require.Equal(t, "linux", resolved.TargetOS)
	require.Equal(t, TriggerModeToggle, resolved.Trigger.Mode)
	require.Equal(t, []string{}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "f12", resolved.Trigger.Hotkey.Key)
	require.Equal(t, AudioBackendPWRecord, resolved.Audio.Backend)
	require.Equal(t, "linux", resolved.Clipboard.TargetOS)
	require.Equal(t, "wl-copy", resolved.Clipboard.PreferredCopy)
	require.Equal(t, PasteMethodWType, resolved.Clipboard.PreferredPaste)
}

func TestResolvePlatform_TriggerOverridesTakePrecedence(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileSteamDeck},
		Trigger: TriggerConfig{
			Mode: TriggerModeHold,
			Hotkey: HotkeyConfig{
				Modifiers: "ctrl+shift",
				Key:       "f13",
			},
		},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, TriggerModeHold, resolved.Trigger.Mode)
	require.Equal(t, []string{"ctrl", "shift"}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "f13", resolved.Trigger.Hotkey.Key)
}

func TestResolvePlatform_InputDeviceOverrideTakesPrecedence(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileLinux},
		Audio: AudioConfig{
			InputDevice: "alsa_input.usb-test",
		},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, "alsa_input.usb-test", resolved.Audio.InputDevice)
}

func TestResolvePlatform_RejectsWrongOSForProfile(t *testing.T) {
	_, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileMacOS},
	}.ResolvePlatform("linux")

	require.EqualError(t, err, `invalid configuration: platform profile "macos" requires darwin but current os is "linux"`)
}
