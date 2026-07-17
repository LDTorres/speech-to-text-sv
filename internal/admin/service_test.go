package admin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTailLogLines_ReturnsLastNLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	logPath, err := LogFilePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o755))
	require.NoError(t, os.WriteFile(logPath, []byte("one\ntwo\nthree\n"), 0o644))

	lines, err := TailLogLines(2)
	require.NoError(t, err)
	require.Equal(t, []string{"two", "three"}, lines)
}
