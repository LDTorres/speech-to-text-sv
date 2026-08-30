package admin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	configpkg "github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/control"
)

var SupportedModels = []string{"tiny", "base", "small"}

type DaemonConfig struct {
	Profile                   string `json:"profile"`
	Model                     string `json:"model"`
	Language                  string `json:"language"`
	PasteEnable               bool   `json:"paste_enable"`
	ExternalControlEnabled    bool   `json:"external_control_enabled"`
	ExternalControlSocketPath string `json:"external_control_socket_path"`
}

type ApplyInput struct {
	Model                  *string
	Language               *string
	PasteEnable            *bool
	ExternalControlEnabled *bool
	SocketPath             *string
}

type ApplyResult struct {
	Config  DaemonConfig  `json:"config"`
	Service ServiceStatus `json:"service"`
}

type modelSource struct {
	revision  string
	checksums map[string]string
}

func GetDaemonConfig(install ServiceInstall) (DaemonConfig, error) {
	values, err := configpkg.ReadEnvFile(install.EnvironmentFile)
	if err != nil {
		return DaemonConfig{}, err
	}

	externalEnabled := false
	if raw := strings.TrimSpace(values["STTD_EXTERNAL_CONTROL_ENABLED"]); raw != "" {
		externalEnabled, err = strconv.ParseBool(raw)
		if err != nil {
			return DaemonConfig{}, fmt.Errorf("parse STTD_EXTERNAL_CONTROL_ENABLED: %w", err)
		}
	}

	socketPath := strings.TrimSpace(values["STTD_EXTERNAL_CONTROL_SOCKET_PATH"])
	if externalEnabled {
		socketPath, err = control.ResolveSocketPath(socketPath)
		if err != nil {
			return DaemonConfig{}, err
		}
	}

	return DaemonConfig{
		Profile:                   valueOrDefault(values["STTD_PLATFORM_PROFILE"], "steam_deck"),
		Model:                     inferModelName(values["STTD_TRANSCRIBE_MODEL_PATH"]),
		Language:                  valueOrDefault(values["STTD_TRANSCRIBE_LANGUAGE"], "es"),
		PasteEnable:               parseBoolDefault(values["STTD_CLIPBOARD_ENABLE_PASTE"], true),
		ExternalControlEnabled:    externalEnabled,
		ExternalControlSocketPath: socketPath,
	}, nil
}

func ApplyDaemonConfig(ctx context.Context, install ServiceInstall, input ApplyInput) (ApplyResult, error) {
	current, err := GetDaemonConfig(install)
	if err != nil {
		return ApplyResult{}, err
	}
	values, err := configpkg.ReadEnvFile(install.EnvironmentFile)
	if err != nil {
		return ApplyResult{}, err
	}
	source := modelSourceFromValues(values)

	updates := map[string]string{}

	if input.Language != nil {
		next := strings.TrimSpace(*input.Language)
		if next != current.Language {
			current.Language = next
			updates["STTD_TRANSCRIBE_LANGUAGE"] = current.Language
		}
	}
	if input.PasteEnable != nil {
		if *input.PasteEnable != current.PasteEnable {
			current.PasteEnable = *input.PasteEnable
			updates["STTD_CLIPBOARD_ENABLE_PASTE"] = strconv.FormatBool(current.PasteEnable)
		}
	}
	if input.ExternalControlEnabled != nil {
		if *input.ExternalControlEnabled != current.ExternalControlEnabled {
			current.ExternalControlEnabled = *input.ExternalControlEnabled
			updates["STTD_EXTERNAL_CONTROL_ENABLED"] = strconv.FormatBool(current.ExternalControlEnabled)
		}
	}
	if input.SocketPath != nil {
		next := strings.TrimSpace(*input.SocketPath)
		if next != current.ExternalControlSocketPath {
			current.ExternalControlSocketPath = next
			updates["STTD_EXTERNAL_CONTROL_SOCKET_PATH"] = current.ExternalControlSocketPath
		}
	}
	if input.Model != nil {
		model := strings.TrimSpace(*input.Model)
		if model != current.Model {
			modelPath, err := ensureModelDownloaded(ctx, install, model, source)
			if err != nil {
				return ApplyResult{}, err
			}
			current.Model = model
			updates["STTD_TRANSCRIBE_MODEL_PATH"] = modelPath
		}
	}

	if current.ExternalControlEnabled {
		socketPath, err := control.ResolveSocketPath(current.ExternalControlSocketPath)
		if err != nil {
			return ApplyResult{}, err
		}
		if socketPath != current.ExternalControlSocketPath {
			updates["STTD_EXTERNAL_CONTROL_SOCKET_PATH"] = socketPath
		}
		current.ExternalControlSocketPath = socketPath
	}

	if len(updates) > 0 {
		if err := configpkg.WriteEnvFile(install.EnvironmentFile, updates); err != nil {
			return ApplyResult{}, err
		}
		if err := RestartService(ctx); err != nil {
			return ApplyResult{}, err
		}
	}

	serviceStatus, err := GetServiceStatus(ctx)
	if err != nil {
		return ApplyResult{}, err
	}

	return ApplyResult{
		Config:  current,
		Service: serviceStatus,
	}, nil
}

func MarshalJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func ensureModelDownloaded(ctx context.Context, install ServiceInstall, model string, source modelSource) (string, error) {
	if !isSupportedModel(model) {
		return "", fmt.Errorf("unsupported model: %s", model)
	}

	modelDir := filepath.Join(install.WorkingDirectory, ".sttd", "models")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return "", fmt.Errorf("create model dir: %w", err)
	}

	modelPath := filepath.Join(modelDir, modelFileName(model))
	if _, err := os.Stat(modelPath); err == nil {
		if err := verifyModelChecksum(modelPath, source.checksums[model]); err != nil {
			return "", err
		}
		return modelPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat model: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelURL(model, source.revision), nil)
	if err != nil {
		return "", fmt.Errorf("build model request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download model: unexpected status %s", resp.Status)
	}

	tempFile := modelPath + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("create temp model file: %w", err)
	}

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tempFile)
		return "", fmt.Errorf("write model: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempFile)
		return "", fmt.Errorf("close model file: %w", err)
	}
	if expectedChecksum := source.checksums[model]; expectedChecksum != "" {
		actualChecksum := fmt.Sprintf("%x", hash.Sum(nil))
		if actualChecksum != expectedChecksum {
			_ = os.Remove(tempFile)
			return "", fmt.Errorf("model checksum mismatch for %s", model)
		}
	}
	if err := os.Rename(tempFile, modelPath); err != nil {
		_ = os.Remove(tempFile)
		return "", fmt.Errorf("move model into place: %w", err)
	}

	return modelPath, nil
}

func isSupportedModel(model string) bool {
	for _, supported := range SupportedModels {
		if supported == model {
			return true
		}
	}
	return false
}

func inferModelName(path string) string {
	switch filepath.Base(path) {
	case "ggml-tiny.bin":
		return "tiny"
	case "ggml-base.bin":
		return "base"
	case "ggml-small.bin":
		return "small"
	default:
		return ""
	}
}

func modelFileName(model string) string {
	return "ggml-" + model + ".bin"
}

func modelURL(model string, revision string) string {
	return "https://huggingface.co/ggerganov/whisper.cpp/resolve/" + revision + "/" + modelFileName(model)
}

func modelSourceFromValues(values map[string]string) modelSource {
	revision := strings.TrimSpace(values["STTD_MODEL_REVISION"])
	if revision == "" {
		revision = "main"
	}

	return modelSource{
		revision: revision,
		checksums: map[string]string{
			"tiny":  strings.TrimSpace(values["STTD_MODEL_SHA256_TINY"]),
			"base":  strings.TrimSpace(values["STTD_MODEL_SHA256_BASE"]),
			"small": strings.TrimSpace(values["STTD_MODEL_SHA256_SMALL"]),
		},
	}
}

func verifyModelChecksum(path string, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open model for checksum: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash model: %w", err)
	}
	if actualChecksum := fmt.Sprintf("%x", hash.Sum(nil)); actualChecksum != expectedChecksum {
		return fmt.Errorf("model checksum mismatch for %s", filepath.Base(path))
	}

	return nil
}

func parseBoolDefault(value string, fallback bool) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return fallback
	}

	return parsed
}

func valueOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
