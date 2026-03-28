package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigLoad_DefaultPlatformProfile_Auto(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, PlatformProfileAuto, cfg.Platform.Profile)
}

func TestConfigLoad_DefaultMacHotkeyValues(t *testing.T) {
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

func TestConfigLoad_SteamDeckTriggerEnvValues(t *testing.T) {
	t.Setenv("STTD_PLATFORM_PROFILE", "steam_deck")
	t.Setenv("STTD_TRIGGER_DEVICE_PATH", "/dev/input/event0")
	t.Setenv("STTD_TRIGGER_EVENT_TYPE", "1")
	t.Setenv("STTD_TRIGGER_EVENT_CODE", "1337")
	t.Setenv("STTD_TRIGGER_ACTIVE_VALUE", "2")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "/dev/input/event0", cfg.Trigger.DevicePath)
	require.Equal(t, uint16(1), cfg.Trigger.EventType)
	require.Equal(t, uint16(1337), cfg.Trigger.EventCode)
	require.Equal(t, int32(2), cfg.Trigger.ActiveValue)
}

func TestConfigLoad_InvalidSteamDeckTriggerConfig_ReturnsError(t *testing.T) {
	_, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileSteamDeck},
		Trigger: TriggerConfig{
			DevicePath: "/dev/input/event0",
			EventType:  1,
		},
	}.ResolvePlatform("linux")

	require.EqualError(t, err, "invalid configuration: steam deck trigger event code must be greater than zero")
}

func TestResolvePlatform_AutoDarwin_UsesMacosDevDefaults(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileAuto},
	}.ResolvePlatform("darwin")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileMacOSDev, resolved.Profile)
	require.Equal(t, "darwin", resolved.TargetOS)
	require.Equal(t, TriggerSourceHotkey, resolved.Trigger.Source)
	require.Equal(t, TriggerModeHold, resolved.Trigger.Mode)
	require.Equal(t, []string{"cmd", "shift"}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "space", resolved.Trigger.Hotkey.Key)
	require.Equal(t, AudioBackendMacOSCapture, resolved.Audio.Backend)
	require.Equal(t, "darwin", resolved.Clipboard.TargetOS)
	require.Equal(t, "pbcopy", resolved.Clipboard.PreferredCopy)
	require.Equal(t, PasteMethodOSAScript, resolved.Clipboard.PreferredPaste)
}

func TestResolvePlatform_MacOSDev_UsesHotkeyAndMacCapture(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileMacOSDev},
	}.ResolvePlatform("darwin")

	require.NoError(t, err)
	require.Equal(t, TriggerSourceHotkey, resolved.Trigger.Source)
	require.Equal(t, TriggerModeHold, resolved.Trigger.Mode)
	require.Equal(t, []string{"cmd", "shift"}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "space", resolved.Trigger.Hotkey.Key)
	require.Equal(t, AudioBackendMacOSCapture, resolved.Audio.Backend)
}

func TestResolvePlatform_AutoLinux_UsesLinuxDesktopDefaults(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileAuto},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileLinuxDesktop, resolved.Profile)
	require.Equal(t, "linux", resolved.TargetOS)
	require.Equal(t, TriggerSourceStub, resolved.Trigger.Source)
	require.Equal(t, TriggerModeHold, resolved.Trigger.Mode)
	require.Equal(t, AudioBackendFile, resolved.Audio.Backend)
	require.Equal(t, "linux", resolved.Clipboard.TargetOS)
	require.Equal(t, "wl-copy", resolved.Clipboard.PreferredCopy)
	require.Equal(t, PasteMethodWType, resolved.Clipboard.PreferredPaste)
}

func TestResolvePlatform_SteamDeck_UsesEvdevAndPWRecordDefaults(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileSteamDeck},
		Trigger: TriggerConfig{
			DevicePath: "/dev/input/event0",
			EventType:  1,
			EventCode:  1337,
		},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileSteamDeck, resolved.Profile)
	require.Equal(t, TriggerSourceSteam, resolved.Trigger.Source)
	require.Equal(t, TriggerModeHold, resolved.Trigger.Mode)
	require.Equal(t, "/dev/input/event0", resolved.Trigger.DevicePath)
	require.Equal(t, uint16(1), resolved.Trigger.EventType)
	require.Equal(t, uint16(1337), resolved.Trigger.EventCode)
	require.Equal(t, int32(1), resolved.Trigger.ActiveValue)
	require.Equal(t, AudioBackendPWRecord, resolved.Audio.Backend)
	require.Equal(t, "linux", resolved.Clipboard.TargetOS)
	require.Equal(t, "wl-copy", resolved.Clipboard.PreferredCopy)
	require.Equal(t, PasteMethodWType, resolved.Clipboard.PreferredPaste)
}

func TestResolvePlatform_SteamDeck_AllowsHotkeyOverride(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileSteamDeck},
		Trigger: TriggerConfig{
			Source: TriggerSourceHotkey,
			Mode:   TriggerModeToggle,
			Hotkey: HotkeyConfig{
				Modifiers: "",
				Key:       "f13",
			},
		},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileSteamDeck, resolved.Profile)
	require.Equal(t, TriggerSourceHotkey, resolved.Trigger.Source)
	require.Equal(t, TriggerModeToggle, resolved.Trigger.Mode)
	require.Equal(t, []string{}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "f13", resolved.Trigger.Hotkey.Key)
}

func TestResolvePlatform_LinuxDesktop_AllowsHotkeyOverride(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileLinuxDesktop},
		Trigger: TriggerConfig{
			Source: TriggerSourceHotkey,
			Mode:   TriggerModeToggle,
			Hotkey: HotkeyConfig{
				Modifiers: "",
				Key:       "f13",
			},
		},
	}.ResolvePlatform("linux")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileLinuxDesktop, resolved.Profile)
	require.Equal(t, TriggerSourceHotkey, resolved.Trigger.Source)
	require.Equal(t, TriggerModeToggle, resolved.Trigger.Mode)
	require.Equal(t, []string{}, resolved.Trigger.Hotkey.Modifiers)
	require.Equal(t, "f13", resolved.Trigger.Hotkey.Key)
}

func TestResolvePlatform_ComponentOverride_TakesPrecedenceOverProfile(t *testing.T) {
	resolved, err := Config{
		Platform: PlatformConfig{Profile: PlatformProfileMacOSDev},
		Trigger: TriggerConfig{
			Source: TriggerSourceStub,
		},
	}.ResolvePlatform("darwin")

	require.NoError(t, err)
	require.Equal(t, PlatformProfileMacOSDev, resolved.Profile)
	require.Equal(t, TriggerSourceStub, resolved.Trigger.Source)
}

func TestResolvePlatform_ProfileDefaults(t *testing.T) {
	tests := []struct {
		name          string
		profile       PlatformProfile
		goos          string
		triggerSource string
		audioBackend  string
		clipboardOS   string
		copyCommand   string
		pasteCommand  string
	}{
		{
			name:          "macos dev",
			profile:       PlatformProfileMacOSDev,
			goos:          "darwin",
			triggerSource: TriggerSourceHotkey,
			audioBackend:  AudioBackendMacOSCapture,
			clipboardOS:   "darwin",
			copyCommand:   "pbcopy",
			pasteCommand:  PasteMethodOSAScript,
		},
		{
			name:          "steam deck",
			profile:       PlatformProfileSteamDeck,
			goos:          "linux",
			triggerSource: TriggerSourceSteam,
			audioBackend:  AudioBackendPWRecord,
			clipboardOS:   "linux",
			copyCommand:   "wl-copy",
			pasteCommand:  PasteMethodWType,
		},
		{
			name:          "linux desktop",
			profile:       PlatformProfileLinuxDesktop,
			goos:          "linux",
			triggerSource: TriggerSourceStub,
			audioBackend:  AudioBackendFile,
			clipboardOS:   "linux",
			copyCommand:   "wl-copy",
			pasteCommand:  PasteMethodWType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Platform: PlatformConfig{Profile: tt.profile},
			}
			if tt.profile == PlatformProfileSteamDeck {
				cfg.Trigger = TriggerConfig{
					DevicePath: "/dev/input/event0",
					EventType:  1,
					EventCode:  1337,
				}
			}

			resolved, err := cfg.ResolvePlatform(tt.goos)

			require.NoError(t, err)
			require.Equal(t, tt.profile, resolved.Profile)
			require.Equal(t, tt.triggerSource, resolved.Trigger.Source)
			require.Equal(t, tt.audioBackend, resolved.Audio.Backend)
			require.Equal(t, tt.clipboardOS, resolved.Clipboard.TargetOS)
			require.Equal(t, tt.copyCommand, resolved.Clipboard.PreferredCopy)
			require.Equal(t, tt.pasteCommand, resolved.Clipboard.PreferredPaste)
		})
	}
}
