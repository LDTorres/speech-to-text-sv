package admin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ServiceName = "speech-to-text.service"

var ErrServiceUnavailable = errors.New("speech-to-text.service is not installed")

type ServiceInstall struct {
	UnitPath         string
	WorkingDirectory string
	EnvironmentFile  string
	ExecStart        string
}

type ServiceStatus struct {
	ActiveState   string `json:"active_state"`
	SubState      string `json:"sub_state"`
	LoadState     string `json:"load_state"`
	UnitFileState string `json:"unit_file_state"`
	Result        string `json:"result"`
}

func LogFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(home, ".local", "state", "sttd", "sttd.log"), nil
}

func DiscoverServiceInstall() (ServiceInstall, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ServiceInstall{}, fmt.Errorf("resolve home dir: %w", err)
	}

	unitPath := filepath.Join(home, ".config", "systemd", "user", ServiceName)
	file, err := os.Open(unitPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ServiceInstall{}, ErrServiceUnavailable
		}
		return ServiceInstall{}, fmt.Errorf("open service unit: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	install := ServiceInstall{UnitPath: unitPath}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "WorkingDirectory="):
			install.WorkingDirectory = strings.TrimSpace(strings.TrimPrefix(line, "WorkingDirectory="))
		case strings.HasPrefix(line, "EnvironmentFile="):
			install.EnvironmentFile = strings.TrimSpace(strings.TrimPrefix(line, "EnvironmentFile="))
		case strings.HasPrefix(line, "ExecStart="):
			install.ExecStart = strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		}
	}
	if err := scanner.Err(); err != nil {
		return ServiceInstall{}, fmt.Errorf("scan service unit: %w", err)
	}

	if install.WorkingDirectory == "" || install.EnvironmentFile == "" {
		return ServiceInstall{}, fmt.Errorf("service unit %q is missing WorkingDirectory or EnvironmentFile", unitPath)
	}

	return install, nil
}

func GetServiceStatus(ctx context.Context) (ServiceStatus, error) {
	cmd := exec.CommandContext(
		ctx,
		"systemctl",
		"--user",
		"show",
		ServiceName,
		"--property=ActiveState,SubState,LoadState,UnitFileState,Result",
		"--value",
	)
	output, err := cmd.Output()
	if err != nil {
		return ServiceStatus{}, fmt.Errorf("systemctl show %s: %w", ServiceName, err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	status := ServiceStatus{}
	if len(lines) > 0 {
		status.ActiveState = lines[0]
	}
	if len(lines) > 1 {
		status.SubState = lines[1]
	}
	if len(lines) > 2 {
		status.LoadState = lines[2]
	}
	if len(lines) > 3 {
		status.UnitFileState = lines[3]
	}
	if len(lines) > 4 {
		status.Result = lines[4]
	}

	return status, nil
}

func RestartService(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "restart", ServiceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart %s: %w (%s)", ServiceName, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func TailLogLines(lines int) ([]string, error) {
	path, err := LogFilePath()
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read log file: %w", err)
	}

	allLines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	filtered := make([]string, 0, len(allLines))
	for _, line := range allLines {
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}

	if lines <= 0 || len(filtered) <= lines {
		return filtered, nil
	}

	return filtered[len(filtered)-lines:], nil
}
