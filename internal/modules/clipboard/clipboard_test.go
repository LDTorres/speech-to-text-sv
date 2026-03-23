package clipboard

import (
	"context"
	"errors"
	"testing"

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
		logger:      zap.NewNop(),
		enablePaste: enablePaste,
		targetOS:    targetOS,
		lookupPath: func(name string) (string, error) {
			switch name {
			case "pbcopy", "osascript", "wl-copy", "wtype", "xdotool":
				return "/usr/bin/" + name, nil
			default:
				return "", errors.New("not found")
			}
		},
		execCommand: func(ctx context.Context, spec commandSpec) error {
			tc.executed = append(tc.executed, spec)
			return nil
		},
	}
	return tc
}
