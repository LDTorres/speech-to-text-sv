package clipboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestClipboard_Copy_SavesTextForPaste(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("linux", true)

	err := clipboard.Copy(context.Background(), "hello deck")

	require.NoError(t, err)
	require.Len(t, clipboard.executed, 1)
	require.Equal(t, "wl-copy", clipboard.executed[0].name)
	require.Equal(t, "hello deck", clipboard.executed[0].stdin)
	require.Equal(t, "hello deck", clipboard.lastText)
}

func TestClipboard_Copy_OnX11_PrefersXclip(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("linux", true)
	clipboard.lookupEnv = func(key string) string {
		switch key {
		case "DISPLAY":
			return ":0"
		case "WAYLAND_DISPLAY":
			return ""
		default:
			return ""
		}
	}

	err := clipboard.Copy(context.Background(), "hello deck")

	require.NoError(t, err)
	require.Len(t, clipboard.executed, 1)
	require.Equal(t, "xclip", clipboard.executed[0].name)
}

func TestClipboard_Paste_WithoutCopy_ReturnsError(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("linux", true)

	err := clipboard.Paste(context.Background())

	require.ErrorIs(t, err, ErrNothingCopied)
}

func TestClipboard_Paste_UsesPasteCommand(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("linux", true)

	require.NoError(t, clipboard.Copy(context.Background(), "hello deck"))
	err := clipboard.Paste(context.Background())

	require.NoError(t, err)
	require.Len(t, clipboard.executed, 2)
	require.Equal(t, "wtype", clipboard.executed[1].name)
	require.Equal(t, []string{"hello deck"}, clipboard.executed[1].args)
}

func TestClipboard_Paste_OnX11_PrefersXdotool(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("linux", true)
	clipboard.lookupEnv = func(key string) string {
		switch key {
		case "DISPLAY":
			return ":0"
		case "WAYLAND_DISPLAY":
			return ""
		default:
			return ""
		}
	}

	require.NoError(t, clipboard.Copy(context.Background(), "hello deck"))
	err := clipboard.Paste(context.Background())

	require.NoError(t, err)
	require.Len(t, clipboard.executed, 2)
	require.Equal(t, "xdotool", clipboard.executed[1].name)
	require.Equal(t, []string{"type", "--clearmodifiers", "--delay", "1", "hello deck"}, clipboard.executed[1].args)
}

func TestClipboard_Copy_OnX11_WithoutXclip_SucceedsWhenDirectTypingIsAvailable(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("linux", true)
	clipboard.lookupEnv = func(key string) string {
		switch key {
		case "DISPLAY":
			return ":0"
		case "WAYLAND_DISPLAY":
			return ""
		default:
			return ""
		}
	}
	clipboard.lookupPath = func(name string) (string, error) {
		switch name {
		case "xdotool", "wl-copy", "wtype":
			return "/usr/bin/" + name, nil
		default:
			return "", errors.New("not found")
		}
	}

	err := clipboard.Copy(context.Background(), "hello deck")

	require.NoError(t, err)
	require.Empty(t, clipboard.executed)
	require.Equal(t, "hello deck", clipboard.lastText)
}

func TestClipboard_Copy_CommandTimeoutReturnsError(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("linux", true)
	clipboard.commandTimeout = 10 * time.Millisecond
	clipboard.execCommand = func(ctx context.Context, _ commandSpec) error {
		<-ctx.Done()
		return ctx.Err()
	}

	err := clipboard.Copy(context.Background(), "timeout me")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRunCommand_WlCopyDoesNotWaitForDetachedOwner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wl-copy")
	script := "#!/bin/sh\n(sleep 1) >&2 &\nexit 0\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	started := time.Now()
	err := runCommand(context.Background(), commandSpec{name: "wl-copy"})

	require.NoError(t, err)
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func TestClipboard_Darwin_UsesPbcopyAndOsascript(t *testing.T) {
	t.Parallel()

	clipboard := newTestClipboard("darwin", true)

	require.NoError(t, clipboard.Copy(context.Background(), "hello mac"))
	require.NoError(t, clipboard.Paste(context.Background()))

	require.Len(t, clipboard.executed, 2)
	require.Equal(t, "pbcopy", clipboard.executed[0].name)
	require.Equal(t, "hello mac", clipboard.executed[0].stdin)
	require.Equal(t, "osascript", clipboard.executed[1].name)
	require.Equal(
		t,
		[]string{"-e", `tell application "System Events" to keystroke "v" using command down`},
		clipboard.executed[1].args,
	)
}

type testClipboard struct {
	*SystemClipboard
	executed []commandSpec
}

func newTestClipboard(targetOS string, enablePaste bool) *testClipboard {
	tc := &testClipboard{}
	tc.SystemClipboard = &SystemClipboard{
		logger:         zap.NewNop(),
		enablePaste:    enablePaste,
		targetOS:       targetOS,
		commandTimeout: 5 * time.Second,
		lookupPath: func(name string) (string, error) {
			switch name {
			case "pbcopy", "osascript", "wl-copy", "wtype", "xdotool", "xclip":
				return "/usr/bin/" + name, nil
			default:
				return "", errors.New("not found")
			}
		},
		lookupEnv: func(string) string {
			return ""
		},
		execCommand: func(ctx context.Context, spec commandSpec) error {
			tc.executed = append(tc.executed, spec)
			return nil
		},
	}
	return tc
}
