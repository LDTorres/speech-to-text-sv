package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func loadDotEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("open env file %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

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
			return fmt.Errorf("parse env file %q line %d: missing '='", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("parse env file %q line %d: empty key", path, lineNumber)
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value = stripEnvValue(strings.TrimSpace(value))
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env var %q from %q line %d: %w", key, path, lineNumber, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file %q: %w", path, err)
	}

	return nil
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
