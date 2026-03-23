package clipboard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sync"

	"go.uber.org/zap"
)

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

type commandExecutor func(ctx context.Context, spec commandSpec) error
type pathLookup func(string) (string, error)

type SystemClipboard struct {
	logger         *zap.Logger
	enablePaste    bool
	targetOS       string
	preferredCopy  string
	preferredPaste string
	lookupPath     pathLookup
	execCommand    commandExecutor

	mu       sync.Mutex
	lastText string
}

func NewSystem(logger *zap.Logger, enablePaste bool) *SystemClipboard {
	return NewSystemForConfig(logger, enablePaste, runtime.GOOS, "", "")
}

func NewSystemForOS(logger *zap.Logger, enablePaste bool, targetOS string) *SystemClipboard {
	return NewSystemForConfig(logger, enablePaste, targetOS, "", "")
}

func NewSystemForConfig(logger *zap.Logger, enablePaste bool, targetOS string, preferredCopy string, preferredPaste string) *SystemClipboard {
	return &SystemClipboard{
		logger:         logger,
		enablePaste:    enablePaste,
		targetOS:       targetOS,
		preferredCopy:  preferredCopy,
		preferredPaste: preferredPaste,
		lookupPath:     exec.LookPath,
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
		return err
	}

	if err := c.execCommand(ctx, spec); err != nil {
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
		return ErrPasteDisabled
	}

	spec, err := c.resolvePasteCommand(text)
	if err != nil {
		return err
	}

	if err := c.execCommand(ctx, spec); err != nil {
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

func runCommand(ctx context.Context, spec commandSpec) error {
	cmd := exec.CommandContext(ctx, spec.name, spec.args...)
	if spec.stdin != "" {
		cmd.Stdin = bytes.NewBufferString(spec.stdin)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (output: %s)", err, string(bytes.TrimSpace(output)))
	}

	return nil
}
