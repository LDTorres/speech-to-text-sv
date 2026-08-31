package admin

import (
	"context"
	"crypto/sha256"
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

const defaultModelRevision = "362722b3fdcd2300b58a8286933ead1c48619667"

const (
	defaultModelSHA256Tiny  = "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21"
	defaultModelSHA256Base  = "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe"
	defaultModelSHA256Small = "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b"
	defaultModelSHA256Large = "64d182b440b98d5203c4f9bd541544d84c605196c4f7b845dfa11fb23594d1e2"
)

type ModelInfo struct {
	Name            string `json:"name"`
	FileName        string `json:"file_name"`
	DisplaySize     string `json:"display_size"`
	SizeBytes       int64  `json:"size_bytes"`
	ResourceWarning string `json:"resource_warning"`
}

func ModelCatalog() []ModelInfo {
	return []ModelInfo{
		{Name: "tiny", FileName: "ggml-tiny.bin", DisplaySize: "75 MB", SizeBytes: 78643200, ResourceWarning: "smallest and fastest model, with lower accuracy"},
		{Name: "base", FileName: "ggml-base.bin", DisplaySize: "142 MB", SizeBytes: 148897792, ResourceWarning: "balanced size and accuracy"},
		{Name: "small", FileName: "ggml-small.bin", DisplaySize: "466 MB", SizeBytes: 488636416, ResourceWarning: "good accuracy with moderate disk and memory usage"},
		{Name: "large", FileName: "ggml-large-v3.bin", DisplaySize: "3.1 GB", SizeBytes: 3328599654, ResourceWarning: "large model: requires several GB of disk space and more memory"},
	}
}

func ModelInfoFor(model string) (ModelInfo, bool) {
	for _, info := range ModelCatalog() {
		if info.Name == model {
			return info, true
		}
	}
	return ModelInfo{}, false
}

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
		Profile:                   valueOrDefault(values["STTD_PLATFORM_PROFILE"], "linux"),
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
		modelPath, err := ensureModelDownloaded(ctx, install, model, source)
		if err != nil {
			return ApplyResult{}, err
		}
		current.Model = model
		if modelPath != strings.TrimSpace(values["STTD_TRANSCRIBE_MODEL_PATH"]) {
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
		original, err := os.ReadFile(install.EnvironmentFile)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("backup environment file: %w", err)
		}
		originalInfo, err := os.Stat(install.EnvironmentFile)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("stat environment file: %w", err)
		}

		if err := configpkg.WriteEnvFile(install.EnvironmentFile, updates); err != nil {
			return ApplyResult{}, err
		}
		if err := RestartService(ctx); err != nil {
			restoreErr := os.WriteFile(install.EnvironmentFile, original, originalInfo.Mode().Perm())
			if restoreErr == nil {
				restoreErr = os.Chmod(install.EnvironmentFile, originalInfo.Mode().Perm())
			}
			if restoreErr != nil {
				return ApplyResult{}, fmt.Errorf("restart service: %w (restore environment file: %w)", err, restoreErr)
			}
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

func ensureModelDownloaded(ctx context.Context, install ServiceInstall, model string, source modelSource) (string, error) {
	if !isSupportedModel(model) {
		return "", fmt.Errorf("unsupported model: %s", model)
	}

	modelDir := filepath.Join(install.WorkingDirectory, ".sttd", "models")
	if err := os.MkdirAll(modelDir, 0o700); err != nil {
		return "", fmt.Errorf("create model dir: %w", err)
	}

	modelPath := filepath.Join(modelDir, modelFileName(model))
	if info, err := os.Stat(modelPath); err == nil {
		if info.IsDir() || info.Size() == 0 {
			return "", fmt.Errorf("model file is empty or not a regular file: %s", modelPath)
		}
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

	tempFile, err := os.CreateTemp(modelDir, ".model-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp model file: %w", err)
	}
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("set temp model permissions: %w", err)
	}
	tempPath := tempFile.Name()
	file := tempFile

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hash), resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("write model: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("close model file: %w", err)
	}
	if expectedChecksum := source.checksums[model]; expectedChecksum != "" {
		actualChecksum := fmt.Sprintf("%x", hash.Sum(nil))
		if actualChecksum != expectedChecksum {
			_ = os.Remove(tempPath)
			return "", fmt.Errorf("model checksum mismatch for %s", model)
		}
	}
	if err := os.Rename(tempPath, modelPath); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("move model into place: %w", err)
	}

	return modelPath, nil
}

func isSupportedModel(model string) bool {
	_, ok := ModelInfoFor(model)
	return ok
}

func inferModelName(path string) string {
	switch filepath.Base(path) {
	case "ggml-tiny.bin":
		return "tiny"
	case "ggml-base.bin":
		return "base"
	case "ggml-small.bin":
		return "small"
	case "ggml-large-v3.bin", "ggml-large.bin":
		return "large"
	default:
		return ""
	}
}

func modelFileName(model string) string {
	if info, ok := ModelInfoFor(model); ok {
		return info.FileName
	}
	return ""
}

func modelURL(model, revision string) string {
	return "https://huggingface.co/ggerganov/whisper.cpp/resolve/" + revision + "/" + modelFileName(model)
}

func modelSourceFromValues(values map[string]string) modelSource {
	revision := strings.TrimSpace(values["STTD_MODEL_REVISION"])
	if revision == "" {
		revision = defaultModelRevision
	}

	checksums := map[string]string{
		"tiny":  defaultModelSHA256Tiny,
		"base":  defaultModelSHA256Base,
		"small": defaultModelSHA256Small,
		"large": defaultModelSHA256Large,
	}
	for model, key := range map[string]string{
		"tiny":  "STTD_MODEL_SHA256_TINY",
		"base":  "STTD_MODEL_SHA256_BASE",
		"small": "STTD_MODEL_SHA256_SMALL",
		"large": "STTD_MODEL_SHA256_LARGE",
	} {
		if checksum := strings.TrimSpace(values[key]); checksum != "" {
			checksums[model] = checksum
		}
	}

	return modelSource{revision: revision, checksums: checksums}
}

func verifyModelChecksum(path, expectedChecksum string) error {
	if expectedChecksum == "" {
		return nil
	}

	file, err := os.Open(path) // #nosec G304 -- the path is the configured local model file
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

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
