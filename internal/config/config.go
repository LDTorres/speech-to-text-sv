package config

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Platform   PlatformConfig
	App        AppConfig
	Trigger    TriggerConfig
	Audio      AudioConfig
	Transcribe TranscribeConfig
	Clipboard  ClipboardConfig
	Notify     NotifyConfig
}

type PlatformProfile string

const (
	PlatformProfileAuto         PlatformProfile = "auto"
	PlatformProfileMacOSDev     PlatformProfile = "macos_dev"
	PlatformProfileSteamDeck    PlatformProfile = "steam_deck"
	PlatformProfileLinuxDesktop PlatformProfile = "linux_desktop"
)

const (
	TriggerSourceStub   = "stub"
	TriggerSourceHotkey = "hotkey"
	TriggerSourceSteam  = "steam"
)

const (
	AudioBackendFile         = "file"
	AudioBackendMacOSCapture = "macos_capture"
	AudioBackendLinuxCapture = "linux_capture"
	AudioBackendPWRecord     = "pw-record"
)

const (
	PasteMethodOSAScript = "osascript"
	PasteMethodWType     = "wtype"
)

type PlatformConfig struct {
	Profile PlatformProfile `envconfig:"PROFILE" default:"auto"`
}

type AppConfig struct {
	Environment     string        `envconfig:"ENV" default:"development"`
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"5s"`
}

type HotkeyConfig struct {
	Modifiers string `envconfig:"MODIFIERS" default:"cmd+shift"`
	Key       string `envconfig:"KEY" default:"space"`
}

type TriggerConfig struct {
	Source          string        `envconfig:"SOURCE"`
	DoubleTapWindow time.Duration `envconfig:"DOUBLE_TAP_WINDOW" default:"400ms"`
	Hotkey          HotkeyConfig
	DevicePath      string `envconfig:"DEVICE_PATH" default:""`
	EventType       uint16 `envconfig:"EVENT_TYPE" default:"0"`
	EventCode       uint16 `envconfig:"EVENT_CODE" default:"0"`
	ActiveValue     int32  `envconfig:"ACTIVE_VALUE" default:"1"`
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
	Language   string        `envconfig:"LANGUAGE" default:"en"`
	Timeout    time.Duration `envconfig:"TIMEOUT" default:"30s"`
}

type ClipboardConfig struct {
	EnablePaste bool `envconfig:"ENABLE_PASTE" default:"true"`
}

type NotifyConfig struct {
	Enabled bool `envconfig:"ENABLED" default:"false"`
}

type ResolvedPlatform struct {
	Profile   PlatformProfile
	TargetOS  string
	Trigger   ResolvedTrigger
	Audio     ResolvedAudio
	Clipboard ResolvedClipboard
}

type ResolvedTrigger struct {
	Source      string
	Hotkey      ResolvedHotkey
	DevicePath  string
	EventType   uint16
	EventCode   uint16
	ActiveValue int32
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

	if c.Trigger.Source != "" && !isValidTriggerSource(c.Trigger.Source) {
		return fmt.Errorf("invalid configuration: unsupported trigger source %q", c.Trigger.Source)
	}

	if _, err := parseHotkey(c.Trigger.Hotkey); err != nil {
		return err
	}

	if c.Trigger.DoubleTapWindow <= 0 {
		return fmt.Errorf("invalid configuration: double tap window must be greater than zero")
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

	return nil
}

func (c Config) ResolvePlatform(goos string) (ResolvedPlatform, error) {
	profile, err := resolveProfile(c.Platform.Profile, goos)
	if err != nil {
		return ResolvedPlatform{}, err
	}

	resolved := defaultsForProfile(profile)

	if c.Trigger.Source != "" {
		resolved.Trigger.Source = c.Trigger.Source
	}

	if resolved.Trigger.Source == TriggerSourceHotkey {
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
	}

	if c.Trigger.DevicePath != "" {
		resolved.Trigger.DevicePath = c.Trigger.DevicePath
	}
	if c.Trigger.EventType != 0 {
		resolved.Trigger.EventType = c.Trigger.EventType
	}
	if c.Trigger.EventCode != 0 {
		resolved.Trigger.EventCode = c.Trigger.EventCode
	}
	if c.Trigger.ActiveValue != 0 {
		resolved.Trigger.ActiveValue = c.Trigger.ActiveValue
	}

	if c.Audio.InputDevice != "" {
		resolved.Audio.InputDevice = c.Audio.InputDevice
	}

	if err := validateResolvedTrigger(profile, resolved.Trigger.Source); err != nil {
		return ResolvedPlatform{}, err
	}

	if err := validateResolvedSteamDeckTrigger(profile, resolved.Trigger); err != nil {
		return ResolvedPlatform{}, err
	}

	return resolved, nil
}

func (c Config) MustResolveCurrentPlatform() (ResolvedPlatform, error) {
	return c.ResolvePlatform(runtime.GOOS)
}

func isValidProfile(profile PlatformProfile) bool {
	switch profile {
	case PlatformProfileAuto, PlatformProfileMacOSDev, PlatformProfileSteamDeck, PlatformProfileLinuxDesktop:
		return true
	default:
		return false
	}
}

func isValidTriggerSource(source string) bool {
	switch source {
	case TriggerSourceStub, TriggerSourceHotkey, TriggerSourceSteam:
		return true
	default:
		return false
	}
}

func resolveProfile(profile PlatformProfile, goos string) (PlatformProfile, error) {
	switch profile {
	case PlatformProfileAuto:
		switch goos {
		case "darwin":
			return PlatformProfileMacOSDev, nil
		case "linux":
			return PlatformProfileLinuxDesktop, nil
		default:
			return "", fmt.Errorf("invalid configuration: platform profile %q is unsupported on os %q", profile, goos)
		}
	case PlatformProfileMacOSDev:
		if goos != "darwin" {
			return "", fmt.Errorf("invalid configuration: platform profile %q requires darwin but current os is %q", profile, goos)
		}
		return profile, nil
	case PlatformProfileSteamDeck, PlatformProfileLinuxDesktop:
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
	case PlatformProfileMacOSDev:
		return ResolvedPlatform{
			Profile:  profile,
			TargetOS: "darwin",
			Trigger: ResolvedTrigger{
				Source: TriggerSourceHotkey,
				Hotkey: ResolvedHotkey{
					Modifiers: []string{"cmd", "shift"},
					Key:       "space",
				},
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
			Profile:  profile,
			TargetOS: "linux",
			Trigger: ResolvedTrigger{
				Source:      TriggerSourceSteam,
				EventType:   1,
				ActiveValue: 1,
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
			Profile:  PlatformProfileLinuxDesktop,
			TargetOS: "linux",
			Trigger: ResolvedTrigger{
				Source: TriggerSourceStub,
			},
			Audio: ResolvedAudio{
				Backend:     AudioBackendFile,
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

func validateResolvedTrigger(profile PlatformProfile, source string) error {
	switch profile {
	case PlatformProfileMacOSDev:
		if source == TriggerSourceHotkey || source == TriggerSourceStub {
			return nil
		}
	case PlatformProfileSteamDeck:
		if source == TriggerSourceSteam || source == TriggerSourceStub {
			return nil
		}
	case PlatformProfileLinuxDesktop:
		if source == TriggerSourceStub {
			return nil
		}
	}

	return fmt.Errorf("invalid configuration: trigger source %q is not supported for platform profile %q", source, profile)
}

func validateResolvedSteamDeckTrigger(profile PlatformProfile, triggerConfig ResolvedTrigger) error {
	if profile != PlatformProfileSteamDeck || triggerConfig.Source != TriggerSourceSteam {
		return nil
	}

	if triggerConfig.DevicePath == "" {
		return fmt.Errorf("invalid configuration: steam deck trigger device path is required")
	}

	if triggerConfig.EventType == 0 {
		return fmt.Errorf("invalid configuration: steam deck trigger event type must be greater than zero")
	}

	if triggerConfig.EventCode == 0 {
		return fmt.Errorf("invalid configuration: steam deck trigger event code must be greater than zero")
	}

	return nil
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
	parts := strings.Split(strings.TrimSpace(value), "+")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid configuration: trigger hotkey modifiers are required")
	}

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

	if len(modifiers) == 0 {
		return nil, fmt.Errorf("invalid configuration: trigger hotkey modifiers are required")
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
		case "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12":
			return key, nil
		}
	}

	return "", fmt.Errorf("invalid configuration: unsupported trigger hotkey key %q", key)
}
