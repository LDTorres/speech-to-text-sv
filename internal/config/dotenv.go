package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func loadDotEnvFile(path string) error {
	values, err := ReadEnvFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env var %q from %q: %w", key, path, err)
		}
	}

	return nil
}

func ReadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
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
	lines := make([]string, 0, len(values))
	seen := map[string]bool{}

	existing, err := os.ReadFile(path)
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

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write env file %q: %w", path, err)
	}

	return nil
}
