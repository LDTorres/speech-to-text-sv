package admin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
	file, err := os.Open(unitPath) // #nosec G304 -- the unit path is derived from the current user's home
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

const (
	maxTailReadBytes = 8 << 20
	defaultTailLines = 1000
)

func TailLogLines(lines int) ([]string, error) {
	path, err := LogFilePath()
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path) // #nosec G304 -- the path is derived from the current user's home
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = file.Close() }()

	if lines <= 0 {
		lines = defaultTailLines
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat log file: %w", err)
	}
	start := info.Size() - maxTailReadBytes
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek log file: %w", err)
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	if start > 0 {
		_ = scanner.Scan()
	}
	filtered := make([]string, 0, lines)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		if len(filtered) == lines {
			copy(filtered, filtered[1:])
			filtered[len(filtered)-1] = line
			continue
		}
		filtered = append(filtered, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan log file: %w", err)
	}

	return filtered, nil
}

func TailJournalLines(ctx context.Context, lines int) ([]string, error) {
	if lines <= 0 {
		lines = defaultTailLines
	}

	//nolint:gosec // journalctl is fixed and the line count is an integer limit
	cmd := exec.CommandContext(
		ctx,
		"journalctl",
		"--user-unit",
		ServiceName,
		"--no-pager",
		"--output=cat",
		fmt.Sprintf("--lines=%d", lines),
	)
	output, err := cmd.Output()
	if err != nil {
		return TailLogLines(lines)
	}
	if len(output) > maxTailReadBytes {
		return nil, errors.New("journal output exceeds maximum size")
	}

	filtered := make([]string, 0, lines)
	for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		if len(filtered) == lines {
			copy(filtered, filtered[1:])
			filtered[len(filtered)-1] = line
			continue
		}
		filtered = append(filtered, line)
	}

	return filtered, nil
}
