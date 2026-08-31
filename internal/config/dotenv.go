package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ReadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path) // #nosec G304 -- the caller selects a local configuration file
	if err != nil {
		return nil, fmt.Errorf("open env file %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parse env file %q line %d: missing '='", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("parse env file %q line %d: empty key", path, lineNumber)
		}

		values[key] = stripEnvValue(strings.TrimSpace(value))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file %q: %w", path, err)
	}

	return values, nil
}

func stripEnvValue(value string) string {
	if len(value) < 2 {
		return value
	}

	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}

	return value
}

func WriteEnvFile(path string, values map[string]string) error {
	for key, value := range values {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\r\n") {
			return fmt.Errorf("invalid env key %q", key)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("invalid value for env key %q: newlines are not allowed", key)
		}
	}

	lines := make([]string, 0, len(values))
	seen := map[string]bool{}

	existing, err := os.ReadFile(path) // #nosec G304 -- the caller selects a local configuration file
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read env file %q: %w", path, err)
	}

	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(existing)))
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				lines = append(lines, line)
				continue
			}

			key, _, ok := strings.Cut(trimmed, "=")
			if !ok {
				lines = append(lines, line)
				continue
			}

			key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
			value, shouldUpdate := values[key]
			if shouldUpdate {
				lines = append(lines, fmt.Sprintf("%s=%s", key, value))
				seen[key] = true
				continue
			}

			lines = append(lines, line)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return fmt.Errorf("scan env file %q: %w", path, scanErr)
		}
	}

	for key, value := range values {
		if seen[key] {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	content := strings.Join(lines, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat env file %q: %w", path, statErr)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary env file: %w", err)
	}
	tempPath := tempFile.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tempPath)
		}
	}()

	if err := tempFile.Chmod(mode); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("set env file permissions: %w", err)
	}
	if _, err := tempFile.WriteString(content); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write env file %q: %w", path, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync env file %q: %w", path, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close env file %q: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace env file %q: %w", path, err)
	}
	renamed = true

	return nil
}
