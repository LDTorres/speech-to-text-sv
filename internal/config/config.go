package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	App       AppConfig
	Server    ServerConfig
	Database  DatabaseConfig
	Telemetry TelemetryConfig
}

type AppConfig struct {
	Environment string `envconfig:"ENV" default:"development"`
}

type ServerConfig struct {
	Addr              string        `envconfig:"ADDR" default:":8080"`
	ReadHeaderTimeout time.Duration `envconfig:"READ_HEADER_TIMEOUT" default:"5s"`
	ReadTimeout       time.Duration `envconfig:"READ_TIMEOUT" default:"10s"`
	WriteTimeout      time.Duration `envconfig:"WRITE_TIMEOUT" default:"10s"`
	IdleTimeout       time.Duration `envconfig:"IDLE_TIMEOUT" default:"60s"`
	RequestTimeout    time.Duration `envconfig:"REQUEST_TIMEOUT" default:"15s"`
	ShutdownTimeout   time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`
}

type DatabaseConfig struct {
	URL string `envconfig:"URL" required:"true"`
}

type TelemetryConfig struct {
	Enabled        bool   `envconfig:"ENABLED" default:"false"`
	ServiceName    string `envconfig:"SERVICE_NAME" default:"golang-api-template"`
	ServiceVersion string `envconfig:"SERVICE_VERSION" default:"0.1.0"`
	Exporter       string `envconfig:"EXPORTER" default:"stdout"`
}

func Load() (Config, error) {
	cfg := Config{}

	if err := envconfig.Process("APP", &cfg.App); err != nil {
		return Config{}, fmt.Errorf("load app config: %w", err)
	}

	if err := envconfig.Process("HTTP", &cfg.Server); err != nil {
		return Config{}, fmt.Errorf("load server config: %w", err)
	}

	if err := envconfig.Process("DATABASE", &cfg.Database); err != nil {
		return Config{}, fmt.Errorf("load database config: %w", err)
	}

	if err := envconfig.Process("OTEL", &cfg.Telemetry); err != nil {
		return Config{}, fmt.Errorf("load telemetry config: %w", err)
	}

	return cfg, nil
}
