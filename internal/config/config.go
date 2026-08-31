package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
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
	AudioWakeAuto            = "auto"
	AudioWakeNone            = "none"
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
	TempDir           string `envconfig:"TEMP_DIR" default:"/tmp/sttd"`
	FileName          string `envconfig:"FILE_NAME" default:"last-recording.wav"`
	SampleFormat      string `envconfig:"SAMPLE_FORMAT" default:"wav"`
	InputDevice       string `envconfig:"INPUT_DEVICE" default:""`
	CameraWake        string `envconfig:"CAMERA_WAKE" default:"auto"`
	CameraVideoDevice string `envconfig:"CAMERA_VIDEO_DEVICE" default:""`
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
	Backend           string
	InputDevice       string
	CameraWake        string
	CameraVideoDevice string
}

type ResolvedClipboard struct {
	TargetOS       string
	PreferredCopy  string
	PreferredPaste string
}

func Load() (Config, error) {
	dotEnvValues, err := ReadEnvFile(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}
	if dotEnvValues == nil {
		dotEnvValues = map[string]string{}
	}

	value := func(key, fallback string) string {
		if processValue, ok := os.LookupEnv(key); ok {
			return processValue
		}
		if fileValue, ok := dotEnvValues[key]; ok {
			return fileValue
		}
		return fallback
	}
	parseDuration := func(key string, fallback time.Duration) (time.Duration, error) {
		raw := value(key, "")
		if strings.TrimSpace(raw) == "" {
			return fallback, nil
		}
		parsed, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}
	parseBool := func(key string, fallback bool) (bool, error) {
		raw := value(key, "")
		if strings.TrimSpace(raw) == "" {
			return fallback, nil
		}
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return false, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}

	appShutdownTimeout, err := parseDuration("STTD_APP_SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("load app config: %w", err)
	}
	triggerDoubleTapWindow, err := parseDuration("STTD_TRIGGER_DOUBLE_TAP_WINDOW", 400*time.Millisecond)
	if err != nil {
		return Config{}, fmt.Errorf("load trigger config: %w", err)
	}
	transcribeTimeout, err := parseDuration("STTD_TRANSCRIBE_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("load transcribe config: %w", err)
	}
	clipboardTimeout, err := parseDuration("STTD_CLIPBOARD_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("load clipboard config: %w", err)
	}
	pasteEnabled, err := parseBool("STTD_CLIPBOARD_ENABLE_PASTE", true)
	if err != nil {
		return Config{}, fmt.Errorf("load clipboard config: %w", err)
	}
	notifyEnabled, err := parseBool("STTD_NOTIFY_ENABLED", false)
	if err != nil {
		return Config{}, fmt.Errorf("load notify config: %w", err)
	}
	externalControlEnabled, err := parseBool("STTD_EXTERNAL_CONTROL_ENABLED", false)
	if err != nil {
		return Config{}, fmt.Errorf("load external control config: %w", err)
	}

	cfg := Config{
		Platform: PlatformConfig{
			Profile:     PlatformProfile(value("STTD_PLATFORM_PROFILE", string(PlatformProfileLinux))),
			Integration: PlatformIntegration(value("STTD_PLATFORM_INTEGRATION", string(PlatformIntegrationNone))),
		},
		App: AppConfig{
			Environment:     value("STTD_APP_ENV", "development"),
			ShutdownTimeout: appShutdownTimeout,
		},
		Trigger: TriggerConfig{
			Mode:            value("STTD_TRIGGER_MODE", ""),
			DoubleTapWindow: triggerDoubleTapWindow,
			Hotkey: HotkeyConfig{
				Modifiers: value("STTD_TRIGGER_HOTKEY_MODIFIERS", ""),
				Key:       value("STTD_TRIGGER_HOTKEY_KEY", ""),
			},
		},
		ExternalControl: ExternalControlConfig{
			Enabled:    externalControlEnabled,
			SocketPath: value("STTD_EXTERNAL_CONTROL_SOCKET_PATH", ""),
			enabledSet: hasValue(dotEnvValues, "STTD_EXTERNAL_CONTROL_ENABLED") || hasEnvKey("STTD_EXTERNAL_CONTROL_ENABLED"),
		},
		Audio: AudioConfig{
			TempDir:           value("STTD_AUDIO_TEMP_DIR", "/tmp/sttd"),
			FileName:          value("STTD_AUDIO_FILE_NAME", "last-recording.wav"),
			SampleFormat:      value("STTD_AUDIO_SAMPLE_FORMAT", "wav"),
			InputDevice:       value("STTD_AUDIO_INPUT_DEVICE", ""),
			CameraWake:        value("STTD_AUDIO_CAMERA_WAKE", "auto"),
			CameraVideoDevice: value("STTD_AUDIO_CAMERA_VIDEO_DEVICE", ""),
		},
		Transcribe: TranscribeConfig{
			BinaryPath: value("STTD_TRANSCRIBE_BINARY_PATH", ""),
			ModelPath:  value("STTD_TRANSCRIBE_MODEL_PATH", ""),
			Language:   value("STTD_TRANSCRIBE_LANGUAGE", "es"),
			Timeout:    transcribeTimeout,
		},
		Clipboard: ClipboardConfig{
			EnablePaste: pasteEnabled,
			Timeout:     clipboardTimeout,
		},
		Notify: NotifyConfig{Enabled: notifyEnabled},
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
	if strings.TrimSpace(c.ExternalControl.SocketPath) != "" {
		if !filepath.IsAbs(strings.TrimSpace(c.ExternalControl.SocketPath)) {
			return fmt.Errorf("invalid configuration: external control socket path must be absolute")
		}
	}

	if strings.TrimSpace(c.Audio.TempDir) == "" {
		return fmt.Errorf("invalid configuration: audio temp dir is required")
	}
	if !filepath.IsAbs(strings.TrimSpace(c.Audio.TempDir)) {
		return fmt.Errorf("invalid configuration: audio temp dir must be absolute")
	}

	fileName := strings.TrimSpace(c.Audio.FileName)
	if fileName == "" {
		return fmt.Errorf("invalid configuration: audio file name is required")
	}
	if filepath.Base(fileName) != fileName || fileName == "." || fileName == ".." {
		return fmt.Errorf("invalid configuration: audio file name must be a simple file name")
	}

	if c.Audio.CameraWake != "" && c.Audio.CameraWake != AudioWakeAuto && c.Audio.CameraWake != AudioWakeNone {
		return fmt.Errorf("invalid configuration: unsupported audio camera wake mode %q", c.Audio.CameraWake)
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
	if c.Audio.CameraWake != "" {
		resolved.Audio.CameraWake = c.Audio.CameraWake
	}
	if c.Audio.CameraVideoDevice != "" {
		resolved.Audio.CameraVideoDevice = strings.TrimSpace(c.Audio.CameraVideoDevice)
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
				Backend:           AudioBackendMacOSCapture,
				InputDevice:       "",
				CameraWake:        AudioWakeNone,
				CameraVideoDevice: "",
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
				Backend:           AudioBackendPWRecord,
				InputDevice:       "",
				CameraWake:        AudioWakeAuto,
				CameraVideoDevice: "",
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
				Backend:           AudioBackendPWRecord,
				InputDevice:       "",
				CameraWake:        AudioWakeAuto,
				CameraVideoDevice: "",
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

func hasValue(values map[string]string, key string) bool {
	_, ok := values[key]
	return ok
}
