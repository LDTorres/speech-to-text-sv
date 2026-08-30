package config

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Platform        PlatformConfig
	App             AppConfig
	Trigger         TriggerConfig
	ExternalControl ExternalControlConfig
	Audio           AudioConfig
	Transcribe      TranscribeConfig
	Clipboard       ClipboardConfig
	Notify          NotifyConfig
}

type PlatformProfile string

const (
	PlatformProfileMacOS     PlatformProfile = "macos"
	PlatformProfileSteamDeck PlatformProfile = "steam_deck"
	PlatformProfileLinux     PlatformProfile = "linux"
)

type PlatformIntegration string

const (
	PlatformIntegrationNone     PlatformIntegration = ""
	PlatformIntegrationHyprland PlatformIntegration = "hyprland"
)

const (
	TriggerModeHold   = "hold"
	TriggerModeToggle = "toggle"
)

const (
	AudioBackendMacOSCapture = "macos_capture"
	AudioBackendPWRecord     = "pw-record"
)

const (
	PasteMethodOSAScript = "osascript"
	PasteMethodWType     = "wtype"
)

type PlatformConfig struct {
	Profile     PlatformProfile     `envconfig:"PROFILE" default:"linux"`
	Integration PlatformIntegration `envconfig:"INTEGRATION" default:""`
}

type AppConfig struct {
	Environment     string        `envconfig:"ENV" default:"development"`
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"5s"`
}

type HotkeyConfig struct {
	Modifiers string `envconfig:"MODIFIERS"`
	Key       string `envconfig:"KEY"`
}

type TriggerConfig struct {
	Mode            string        `envconfig:"MODE"`
	DoubleTapWindow time.Duration `envconfig:"DOUBLE_TAP_WINDOW" default:"400ms"`
	Hotkey          HotkeyConfig
}

type ExternalControlConfig struct {
	Enabled    bool   `envconfig:"ENABLED"`
	SocketPath string `envconfig:"SOCKET_PATH" default:""`

	enabledSet bool
}

type AudioConfig struct {
	TempDir      string `envconfig:"TEMP_DIR" default:"/tmp/sttd"`
	FileName     string `envconfig:"FILE_NAME" default:"last-recording.wav"`
	SampleFormat string `envconfig:"SAMPLE_FORMAT" default:"wav"`
	InputDevice  string `envconfig:"INPUT_DEVICE" default:""`
}

type TranscribeConfig struct {
	BinaryPath string        `envconfig:"BINARY_PATH" default:""`
	ModelPath  string        `envconfig:"MODEL_PATH" default:""`
	Language   string        `envconfig:"LANGUAGE" default:"es"`
	Timeout    time.Duration `envconfig:"TIMEOUT" default:"30s"`
}

type ClipboardConfig struct {
	EnablePaste bool          `envconfig:"ENABLE_PASTE" default:"true"`
	Timeout     time.Duration `envconfig:"TIMEOUT" default:"5s"`
}

type NotifyConfig struct {
	Enabled bool `envconfig:"ENABLED" default:"false"`
}

type ResolvedPlatform struct {
	Profile         PlatformProfile
	Integration     PlatformIntegration
	TargetOS        string
	Trigger         ResolvedTrigger
	ExternalControl ResolvedExternalControl
	Audio           ResolvedAudio
	Clipboard       ResolvedClipboard
}

type ResolvedTrigger struct {
	Mode   string
	Hotkey ResolvedHotkey
}

type ResolvedExternalControl struct {
	Enabled    bool
	SocketPath string
}

type ResolvedHotkey struct {
	Modifiers []string
	Key       string
}

type ResolvedAudio struct {
	Backend     string
	InputDevice string
}

type ResolvedClipboard struct {
	TargetOS       string
	PreferredCopy  string
	PreferredPaste string
}

func Load() (Config, error) {
	cfg := Config{}

	if err := loadDotEnvFile(".env"); err != nil {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	if err := envconfig.Process("STTD_PLATFORM", &cfg.Platform); err != nil {
		return Config{}, fmt.Errorf("load platform config: %w", err)
	}

	if err := envconfig.Process("STTD_APP", &cfg.App); err != nil {
		return Config{}, fmt.Errorf("load app config: %w", err)
	}

	if err := envconfig.Process("STTD_TRIGGER", &cfg.Trigger); err != nil {
		return Config{}, fmt.Errorf("load trigger config: %w", err)
	}

	if err := envconfig.Process("STTD_TRIGGER_HOTKEY", &cfg.Trigger.Hotkey); err != nil {
		return Config{}, fmt.Errorf("load trigger hotkey config: %w", err)
	}

	cfg.ExternalControl.enabledSet = hasEnvKey("STTD_EXTERNAL_CONTROL_ENABLED")
	if err := envconfig.Process("STTD_EXTERNAL_CONTROL", &cfg.ExternalControl); err != nil {
		return Config{}, fmt.Errorf("load external control config: %w", err)
	}

	if err := envconfig.Process("STTD_AUDIO", &cfg.Audio); err != nil {
		return Config{}, fmt.Errorf("load audio config: %w", err)
	}

	if err := envconfig.Process("STTD_TRANSCRIBE", &cfg.Transcribe); err != nil {
		return Config{}, fmt.Errorf("load transcribe config: %w", err)
	}

	if err := envconfig.Process("STTD_CLIPBOARD", &cfg.Clipboard); err != nil {
		return Config{}, fmt.Errorf("load clipboard config: %w", err)
	}

	if err := envconfig.Process("STTD_NOTIFY", &cfg.Notify); err != nil {
		return Config{}, fmt.Errorf("load notify config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	if c.App.ShutdownTimeout <= 0 {
		return fmt.Errorf("invalid configuration: shutdown timeout must be greater than zero")
	}

	if !isValidProfile(c.Platform.Profile) {
		return fmt.Errorf("invalid configuration: unsupported platform profile %q", c.Platform.Profile)
	}

	if !isValidIntegration(c.Platform.Integration) {
		return fmt.Errorf("invalid configuration: unsupported platform integration %q", c.Platform.Integration)
	}

	if c.Platform.Integration == PlatformIntegrationHyprland && c.Platform.Profile == PlatformProfileMacOS {
		return fmt.Errorf("invalid configuration: hyprland integration requires a linux profile")
	}

	if c.Trigger.Mode != "" && !isValidTriggerMode(c.Trigger.Mode) {
		return fmt.Errorf("invalid configuration: unsupported trigger mode %q", c.Trigger.Mode)
	}

	if c.Trigger.Hotkey.Modifiers != "" || c.Trigger.Hotkey.Key != "" {
		if _, err := parseHotkey(c.Trigger.Hotkey); err != nil {
			return err
		}
	}

	if c.Trigger.DoubleTapWindow <= 0 {
		return fmt.Errorf("invalid configuration: double tap window must be greater than zero")
	}

	if c.ExternalControl.SocketPath != "" && strings.TrimSpace(c.ExternalControl.SocketPath) == "" {
		return fmt.Errorf("invalid configuration: external control socket path must not be blank")
	}

	if c.Audio.TempDir == "" {
		return fmt.Errorf("invalid configuration: audio temp dir is required")
	}

	if c.Audio.FileName == "" {
		return fmt.Errorf("invalid configuration: audio file name is required")
	}

	if c.Transcribe.Timeout <= 0 {
		return fmt.Errorf("invalid configuration: transcribe timeout must be greater than zero")
	}

	if c.Clipboard.Timeout <= 0 {
		return fmt.Errorf("invalid configuration: clipboard timeout must be greater than zero")
	}

	return nil
}

func (c Config) ResolvePlatform(goos string) (ResolvedPlatform, error) {
	profile, err := resolveProfile(c.Platform.Profile, goos)
	if err != nil {
		return ResolvedPlatform{}, err
	}

	resolved := defaultsForProfile(profile)
	if c.Platform.Integration != PlatformIntegrationNone {
		resolved.Integration = c.Platform.Integration
	}
	if resolved.Integration == PlatformIntegrationHyprland {
		resolved.ExternalControl.Enabled = true
	}

	if c.Trigger.Mode != "" {
		resolved.Trigger.Mode = c.Trigger.Mode
	}

	hotkeyCfg := HotkeyConfig{
		Modifiers: strings.Join(resolved.Trigger.Hotkey.Modifiers, "+"),
		Key:       resolved.Trigger.Hotkey.Key,
	}
	if c.Trigger.Hotkey.Modifiers != "" {
		hotkeyCfg.Modifiers = c.Trigger.Hotkey.Modifiers
	}
	if c.Trigger.Hotkey.Key != "" {
		hotkeyCfg.Key = c.Trigger.Hotkey.Key
	}

	hotkey, err := parseHotkey(hotkeyCfg)
	if err != nil {
		return ResolvedPlatform{}, err
	}
	resolved.Trigger.Hotkey = hotkey

	if c.Audio.InputDevice != "" {
		resolved.Audio.InputDevice = c.Audio.InputDevice
	}

	if c.ExternalControl.enabledSet {
		resolved.ExternalControl.Enabled = c.ExternalControl.Enabled
	}
	if c.ExternalControl.SocketPath != "" {
		resolved.ExternalControl.SocketPath = strings.TrimSpace(c.ExternalControl.SocketPath)
	}

	return resolved, nil
}

func (c Config) MustResolveCurrentPlatform() (ResolvedPlatform, error) {
	return c.ResolvePlatform(runtime.GOOS)
}

func isValidProfile(profile PlatformProfile) bool {
	switch profile {
	case PlatformProfileMacOS, PlatformProfileSteamDeck, PlatformProfileLinux:
		return true
	default:
		return false
	}
}

func isValidIntegration(integration PlatformIntegration) bool {
	switch integration {
	case PlatformIntegrationNone, PlatformIntegrationHyprland:
		return true
	default:
		return false
	}
}

func isValidTriggerMode(mode string) bool {
	switch mode {
	case TriggerModeHold, TriggerModeToggle:
		return true
	default:
		return false
	}
}

func resolveProfile(profile PlatformProfile, goos string) (PlatformProfile, error) {
	switch profile {
	case PlatformProfileMacOS:
		if goos != "darwin" {
			return "", fmt.Errorf("invalid configuration: platform profile %q requires darwin but current os is %q", profile, goos)
		}
		return profile, nil
	case PlatformProfileSteamDeck, PlatformProfileLinux:
		if goos != "linux" {
			return "", fmt.Errorf("invalid configuration: platform profile %q requires linux but current os is %q", profile, goos)
		}
		return profile, nil
	default:
		return "", fmt.Errorf("invalid configuration: unsupported platform profile %q", profile)
	}
}

func defaultsForProfile(profile PlatformProfile) ResolvedPlatform {
	switch profile {
	case PlatformProfileMacOS:
		return ResolvedPlatform{
			Profile:     profile,
			Integration: PlatformIntegrationNone,
			TargetOS:    "darwin",
			Trigger: ResolvedTrigger{
				Mode: TriggerModeHold,
				Hotkey: ResolvedHotkey{
					Modifiers: []string{"cmd", "shift"},
					Key:       "space",
				},
			},
			ExternalControl: ResolvedExternalControl{
				Enabled: false,
			},
			Audio: ResolvedAudio{
				Backend:     AudioBackendMacOSCapture,
				InputDevice: "",
			},
			Clipboard: ResolvedClipboard{
				TargetOS:       "darwin",
				PreferredCopy:  "pbcopy",
				PreferredPaste: PasteMethodOSAScript,
			},
		}
	case PlatformProfileSteamDeck:
		return ResolvedPlatform{
			Profile:     profile,
			Integration: PlatformIntegrationNone,
			TargetOS:    "linux",
			Trigger: ResolvedTrigger{
				Mode: TriggerModeToggle,
				Hotkey: ResolvedHotkey{
					Modifiers: []string{},
					Key:       "f12",
				},
			},
			ExternalControl: ResolvedExternalControl{
				Enabled: true,
			},
			Audio: ResolvedAudio{
				Backend:     AudioBackendPWRecord,
				InputDevice: "",
			},
			Clipboard: ResolvedClipboard{
				TargetOS:       "linux",
				PreferredCopy:  "wl-copy",
				PreferredPaste: PasteMethodWType,
			},
		}
	default:
		return ResolvedPlatform{
			Profile:     PlatformProfileLinux,
			Integration: PlatformIntegrationNone,
			TargetOS:    "linux",
			Trigger: ResolvedTrigger{
				Mode: TriggerModeHold,
				Hotkey: ResolvedHotkey{
					Modifiers: []string{"ctrl", "shift"},
					Key:       "space",
				},
			},
			ExternalControl: ResolvedExternalControl{
				Enabled: false,
			},
			Audio: ResolvedAudio{
				Backend:     AudioBackendPWRecord,
				InputDevice: "",
			},
			Clipboard: ResolvedClipboard{
				TargetOS:       "linux",
				PreferredCopy:  "wl-copy",
				PreferredPaste: PasteMethodWType,
			},
		}
	}
}

func parseHotkey(cfg HotkeyConfig) (ResolvedHotkey, error) {
	modifiers, err := normalizeHotkeyModifiers(cfg.Modifiers)
	if err != nil {
		return ResolvedHotkey{}, err
	}

	key, err := normalizeHotkeyKey(cfg.Key)
	if err != nil {
		return ResolvedHotkey{}, err
	}

	return ResolvedHotkey{
		Modifiers: modifiers,
		Key:       key,
	}, nil
}

func normalizeHotkeyModifiers(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return []string{}, nil
	}

	parts := strings.Split(strings.TrimSpace(value), "+")
	seen := map[string]bool{}
	modifiers := make([]string, 0, len(parts))

	for _, part := range parts {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			return nil, fmt.Errorf("invalid configuration: trigger hotkey modifiers contain an empty value")
		}

		switch token {
		case "cmd", "shift", "ctrl", "alt":
		default:
			return nil, fmt.Errorf("invalid configuration: unsupported trigger hotkey modifier %q", token)
		}

		if seen[token] {
			continue
		}

		seen[token] = true
		modifiers = append(modifiers, token)
	}

	return modifiers, nil
}

func normalizeHotkeyKey(value string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		return "", fmt.Errorf("invalid configuration: trigger hotkey key is required")
	}

	if key == "space" || key == "tab" || key == "return" || key == "escape" {
		return key, nil
	}

	if len(key) == 1 {
		ch := key[0]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			return key, nil
		}
	}

	if strings.HasPrefix(key, "f") {
		switch key {
		case "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12", "f13", "f14", "f15", "f16", "f17", "f18", "f19", "f20":
			return key, nil
		}
	}

	return "", fmt.Errorf("invalid configuration: unsupported trigger hotkey key %q", key)
}

func hasEnvKey(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}
