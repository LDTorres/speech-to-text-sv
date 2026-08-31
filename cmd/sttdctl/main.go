package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/admin"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/control"
)

const (
	socketTimeout         = 5 * time.Second
	configApplyTimeout    = 10 * time.Minute
	serviceCommandTimeout = 30 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		writeError(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sttdctl <control|config|service|model|doctor> ...")
	}

	switch args[0] {
	case "control":
		return runControl(ctx, args[1:])
	case "config":
		return runConfig(ctx, args[1:])
	case "service":
		return runService(ctx, args[1:])
	case "model":
		return runModel(args[1:])
	case "doctor":
		return runDoctor(ctx, args[1:])
	case "logs":
		return runLogs(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command group %q", args[0])
	}
}

func runModel(args []string) error {
	if len(args) < 1 || args[0] != "list" {
		return fmt.Errorf("usage: sttdctl model list [--json]")
	}

	fs := flag.NewFlagSet("model list", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	catalog := admin.ModelCatalog()
	if *jsonOut {
		return writeJSON(catalog)
	}
	for _, info := range catalog {
		fmt.Printf("%s - %s - %s\n", info.Name, info.DisplaySize, info.ResourceWarning)
	}
	return nil
}

func runDoctor(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: sttdctl doctor")
	}

	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve sttdctl path: %w", err)
	}
	doctorPath := filepath.Join(filepath.Dir(executablePath), "doctor.sh")
	cmd := exec.CommandContext(ctx, doctorPath, "--status") // #nosec G204 -- doctorPath is derived from the executable directory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run doctor: %w", err)
	}

	return nil
}

func runControl(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sttdctl control <ping|status|start|stop|toggle|retry> [--json]")
	}

	command := args[0]
	fs := flag.NewFlagSet("control", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	install, err := admin.DiscoverServiceInstall()
	if err != nil {
		return err
	}
	cfg, err := admin.GetDaemonConfig(install)
	if err != nil {
		return err
	}
	if !cfg.ExternalControlEnabled {
		return errors.New("external control is disabled")
	}

	socketPath := cfg.ExternalControlSocketPath
	if strings.TrimSpace(socketPath) == "" {
		socketPath, err = control.ResolveSocketPath("")
		if err != nil {
			return err
		}
	}

	response, err := sendControlRequest(ctx, socketPath, control.Request{
		Command: command,
		Source:  "sttdctl",
	})
	if err != nil {
		return err
	}

	if *jsonOut {
		if err := writeJSON(response); err != nil {
			return err
		}
		if !response.OK {
			return responseError(response)
		}
		return nil
	}

	if !response.OK {
		return responseError(response)
	}
	fmt.Println(response.Message)
	return nil
}

func responseError(response control.Response) error {
	message := strings.TrimSpace(response.Message)
	if message == "" {
		message = "control request failed"
	}
	if code := strings.TrimSpace(response.ErrorCode); code != "" {
		return fmt.Errorf("%s: %s", code, message)
	}
	return errors.New(message)
}

func runConfig(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sttdctl config <get|apply> [--json]")
	}

	switch args[0] {
	case "get":
		fs := flag.NewFlagSet("config get", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		install, err := admin.DiscoverServiceInstall()
		if err != nil {
			return err
		}
		cfg, err := admin.GetDaemonConfig(install)
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(cfg)
		}
		fmt.Printf("%+v\n", cfg)
		return nil
	case "apply":
		fs := flag.NewFlagSet("config apply", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		model := fs.String("model", "", "model")
		language := fs.String("language", "", "language")
		pasteEnable := fs.String("paste-enable", "", "true or false")
		externalEnabled := fs.String("external-control-enabled", "", "true or false")
		socketPath := fs.String("external-control-socket-path", "", "socket path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		install, err := admin.DiscoverServiceInstall()
		if err != nil {
			return err
		}

		input := admin.ApplyInput{}
		if strings.TrimSpace(*model) != "" {
			input.Model = model
		}
		if strings.TrimSpace(*language) != "" {
			input.Language = language
		}
		if strings.TrimSpace(*pasteEnable) != "" {
			value, parseErr := parseBoolFlag(*pasteEnable)
			if parseErr != nil {
				return parseErr
			}
			input.PasteEnable = &value
		}
		if strings.TrimSpace(*externalEnabled) != "" {
			value, parseErr := parseBoolFlag(*externalEnabled)
			if parseErr != nil {
				return parseErr
			}
			input.ExternalControlEnabled = &value
		}
		if *socketPath != "" {
			input.SocketPath = socketPath
		}

		applyCtx, cancel := context.WithTimeout(ctx, configApplyTimeout)
		defer cancel()
		result, err := admin.ApplyDaemonConfig(applyCtx, install, input)
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(result)
		}
		fmt.Printf("%+v\n", result)
		return nil
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func runService(ctx context.Context, args []string) error {
	serviceCtx, cancel := context.WithTimeout(ctx, serviceCommandTimeout)
	defer cancel()

	if len(args) < 1 {
		return fmt.Errorf("usage: sttdctl service <status|restart> [--json]")
	}

	switch args[0] {
	case "status":
		fs := flag.NewFlagSet("service status", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		status, err := admin.GetServiceStatus(serviceCtx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(status)
		}
		fmt.Printf("%+v\n", status)
		return nil
	case "restart":
		fs := flag.NewFlagSet("service restart", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := admin.RestartService(serviceCtx); err != nil {
			return err
		}
		status, err := admin.GetServiceStatus(serviceCtx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(status)
		}
		fmt.Printf("%+v\n", status)
		return nil
	default:
		return fmt.Errorf("unknown service command %q", args[0])
	}
}

func runLogs(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sttdctl logs <tail|path> [--json]")
	}

	switch args[0] {
	case "tail":
		fs := flag.NewFlagSet("logs tail", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		lines := fs.Int("lines", 200, "number of lines")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		tail, err := admin.TailJournalLines(ctx, *lines)
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(map[string]any{
				"path":  mustLogPath(),
				"lines": tail,
			})
		}
		for _, line := range tail {
			fmt.Println(line)
		}
		return nil
	case "path":
		fs := flag.NewFlagSet("logs path", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		path, err := admin.LogFilePath()
		if err != nil {
			return err
		}
		if *jsonOut {
			return writeJSON(map[string]string{"path": path})
		}
		fmt.Println(path)
		return nil
	default:
		return fmt.Errorf("unknown logs command %q", args[0])
	}
}

func sendControlRequest(ctx context.Context, socketPath string, request control.Request) (control.Response, error) {
	dialer := net.Dialer{Timeout: socketTimeout}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return control.Response{}, fmt.Errorf("dial external control: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	deadline := time.Now().Add(socketTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return control.Response{}, fmt.Errorf("set control connection deadline: %w", err)
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		return control.Response{}, fmt.Errorf("encode control request: %w", err)
	}
	if _, err := conn.Write(encoded); err != nil {
		return control.Response{}, fmt.Errorf("write control request: %w", err)
	}
	if unixConn, ok := conn.(*net.UnixConn); ok {
		_ = unixConn.CloseWrite()
	}

	var response control.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return control.Response{}, fmt.Errorf("decode control response: %w", err)
	}

	return response, nil
}

func parseBoolFlag(value string) (bool, error) {
	normalized := strings.TrimSpace(value)
	switch strings.ToLower(normalized) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", value)
	}
}

func writeJSON(v any) error {
	encoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func writeError(err error) {
	payload := map[string]string{
		"error": err.Error(),
	}
	encoded, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return
	}
	fmt.Fprintln(os.Stderr, string(encoded))
}

func mustLogPath() string {
	path, err := admin.LogFilePath()
	if err != nil {
		return ""
	}
	return path
}
