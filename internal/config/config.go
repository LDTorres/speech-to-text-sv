package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App        AppConfig
	Trigger    TriggerConfig
	Audio      AudioConfig
	Transcribe TranscribeConfig
	Clipboard  ClipboardConfig
	Notify     NotifyConfig
}

type AppConfig struct {
	Environment     string        `envconfig:"ENV" default:"development"`
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"5s"`
}

type TriggerConfig struct {
	Source          string        `envconfig:"SOURCE" default:"stub"`
	DoubleTapWindow time.Duration `envconfig:"DOUBLE_TAP_WINDOW" default:"400ms"`
}

type AudioConfig struct {
	TempDir      string `envconfig:"TEMP_DIR" default:"/tmp/sttd"`
	FileName     string `envconfig:"FILE_NAME" default:"last-recording.wav"`
	SampleFormat string `envconfig:"SAMPLE_FORMAT" default:"wav"`
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

func Load() (Config, error) {
	cfg := Config{}

	if err := envconfig.Process("STTD_APP", &cfg.App); err != nil {
		return Config{}, fmt.Errorf("load app config: %w", err)
	}

	if err := envconfig.Process("STTD_TRIGGER", &cfg.Trigger); err != nil {
		return Config{}, fmt.Errorf("load trigger config: %w", err)
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
