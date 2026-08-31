package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"
)

const defaultCommandTimeout = 5 * time.Second

var (
	ErrNothingCopied    = errors.New("clipboard does not contain any text")
	ErrPasteDisabled    = errors.New("paste injection disabled")
	ErrCopyUnavailable  = errors.New("clipboard copy command unavailable")
	ErrPasteUnavailable = errors.New("clipboard paste command unavailable")
)

type Clipboard interface {
	Copy(ctx context.Context, text string) error
	Paste(ctx context.Context) error
}

type commandSpec struct {
	name  string
	args  []string
	stdin string
}

type (
	commandExecutor func(ctx context.Context, spec commandSpec) error
	pathLookup      func(string) (string, error)
	envLookup       func(string) string
)

type SystemClipboard struct {
	logger         *zap.Logger
	enablePaste    bool
	targetOS       string
	preferredCopy  string
	preferredPaste string
	commandTimeout time.Duration
	lookupPath     pathLookup
	lookupEnv      envLookup
	execCommand    commandExecutor

	mu       sync.Mutex
	lastText string
}

func NewSystem(logger *zap.Logger, enablePaste bool) *SystemClipboard {
	return NewSystemForConfig(logger, enablePaste, defaultCommandTimeout, runtime.GOOS, "", "")
}

func NewSystemForOS(logger *zap.Logger, enablePaste bool, targetOS string) *SystemClipboard {
	return NewSystemForConfig(logger, enablePaste, defaultCommandTimeout, targetOS, "", "")
}

func NewSystemForConfig(logger *zap.Logger, enablePaste bool, commandTimeout time.Duration, targetOS, preferredCopy, preferredPaste string) *SystemClipboard {
	if commandTimeout <= 0 {
		commandTimeout = defaultCommandTimeout
	}

	return &SystemClipboard{
		logger:         logger,
		enablePaste:    enablePaste,
		targetOS:       targetOS,
		preferredCopy:  preferredCopy,
		preferredPaste: preferredPaste,
		commandTimeout: commandTimeout,
		lookupPath:     exec.LookPath,
		lookupEnv:      os.Getenv,
		execCommand:    runCommand,
	}
}

func (c *SystemClipboard) Copy(ctx context.Context, text string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	c.lastText = text
	c.mu.Unlock()

	spec, err := c.resolveCopyCommand(text)
	if err != nil {
		if errors.Is(err, ErrCopyUnavailable) && c.canTypeDirectly() {
			c.logger.Info("clipboard copy skipped; direct typing paste is available", zap.Int("text_length", len(text)))
			return nil
		}
		return err
	}

	if err := c.execute(ctx, spec); err != nil {
		return fmt.Errorf("execute clipboard copy: %w", err)
	}

	c.logger.Info("clipboard copy attempted", zap.Int("text_length", len(text)), zap.String("command", spec.name))

	return nil
}

func (c *SystemClipboard) Paste(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	text := c.lastText
	c.mu.Unlock()

	if text == "" {
		return ErrNothingCopied
	}

	if !c.enablePaste {
		c.logger.Info("clipboard paste skipped; paste injection is disabled", zap.Int("text_length", len(text)))
		return nil
	}

	spec, err := c.resolvePasteCommand(text)
	if err != nil {
		return err
	}

	if err := c.execute(ctx, spec); err != nil {
		return fmt.Errorf("execute clipboard paste: %w", err)
	}

	c.logger.Info("clipboard paste attempted", zap.Int("text_length", len(text)), zap.String("command", spec.name))

	return nil
}

func (c *SystemClipboard) resolveCopyCommand(text string) (commandSpec, error) {
	if c.targetOS == "darwin" {
		if _, err := c.lookupPath("pbcopy"); err == nil {
			return commandSpec{name: "pbcopy", stdin: text}, nil
		}

		return commandSpec{}, ErrCopyUnavailable
	}

	if c.shouldPreferX11Clipboard() {
		if _, err := c.lookupPath("xclip"); err == nil {
			return commandSpec{
				name:  "xclip",
				args:  []string{"-selection", "clipboard", "-in"},
				stdin: text,
			}, nil
		}

		return commandSpec{}, ErrCopyUnavailable
	}

	if c.preferredCopy == "xclip" {
		if _, err := c.lookupPath("xclip"); err == nil {
			return commandSpec{
				name:  "xclip",
				args:  []string{"-selection", "clipboard", "-in"},
				stdin: text,
			}, nil
		}
	}

	if _, err := c.lookupPath("wl-copy"); err == nil {
		return commandSpec{name: "wl-copy", stdin: text}, nil
	}

	if _, err := c.lookupPath("xclip"); err == nil {
		return commandSpec{
			name:  "xclip",
			args:  []string{"-selection", "clipboard", "-in"},
			stdin: text,
		}, nil
	}

	return commandSpec{}, ErrCopyUnavailable
}

func (c *SystemClipboard) resolvePasteCommand(text string) (commandSpec, error) {
	if c.targetOS == "darwin" {
		if _, err := c.lookupPath("osascript"); err == nil {
			return commandSpec{
				name: "osascript",
				args: []string{
					"-e",
					`tell application "System Events" to keystroke "v" using command down`,
				},
			}, nil
		}

		return commandSpec{}, ErrPasteUnavailable
	}

	if c.shouldPreferX11Clipboard() {
		if _, err := c.lookupPath("xdotool"); err == nil {
			return commandSpec{
				name: "xdotool",
				args: []string{"type", "--clearmodifiers", "--delay", "1", text},
			}, nil
		}
	}

	if c.preferredPaste == "xdotool" {
		if _, err := c.lookupPath("xdotool"); err == nil {
			return commandSpec{
				name: "xdotool",
				args: []string{"key", "--clearmodifiers", "ctrl+v"},
			}, nil
		}
	}

	if _, err := c.lookupPath("wtype"); err == nil {
		return commandSpec{
			name: "wtype",
			args: []string{text},
		}, nil
	}

	if _, err := c.lookupPath("xdotool"); err == nil {
		return commandSpec{
			name: "xdotool",
			args: []string{"key", "--clearmodifiers", "ctrl+v"},
		}, nil
	}

	return commandSpec{}, ErrPasteUnavailable
}

func (c *SystemClipboard) shouldPreferX11Clipboard() bool {
	if c.lookupEnv == nil {
		return false
	}

	display := c.lookupEnv("DISPLAY")
	waylandDisplay := c.lookupEnv("WAYLAND_DISPLAY")

	return display != "" && waylandDisplay == ""
}

func (c *SystemClipboard) canTypeDirectly() bool {
	if !c.shouldPreferX11Clipboard() {
		return false
	}

	_, err := c.lookupPath("xdotool")
	return err == nil
}

func (c *SystemClipboard) execute(ctx context.Context, spec commandSpec) error {
	timeout := c.commandTimeout
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}

	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.execCommand(commandCtx, spec)
}

func runCommand(ctx context.Context, spec commandSpec) error {
	cmd := exec.CommandContext(ctx, spec.name, spec.args...) // #nosec G204 -- command specs come from an internal allowlist
	if spec.stdin != "" {
		cmd.Stdin = bytes.NewBufferString(spec.stdin)
	}

	if spec.name == "wl-copy" {
		// wl-copy intentionally keeps a background process alive as the
		// Wayland clipboard owner. Do not give it pipes used by
		// CombinedOutput: the child inherits them and makes the caller wait
		// until the clipboard is replaced.
		if err := cmd.Run(); err != nil {
			return err
		}
		return nil
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, string(bytes.TrimSpace(output)))
	}

	return nil
}
