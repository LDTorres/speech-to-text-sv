package admin

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelSourceFromValues_UsesConfiguredRevisionAndChecksums(t *testing.T) {
	source := modelSourceFromValues(map[string]string{
		"STTD_MODEL_REVISION":     "frozen-revision",
		"STTD_MODEL_SHA256_SMALL": "abc123",
	})

	require.Equal(t, "frozen-revision", source.revision)
	require.Equal(t, "abc123", source.checksums["small"])
	require.Equal(t, defaultModelSHA256Base, source.checksums["base"])
}

func TestVerifyModelChecksum_RejectsUnexpectedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ggml-base.bin")
	require.NoError(t, os.WriteFile(path, []byte("model"), 0o600))

	expected := sha256.Sum256([]byte("other-model"))
	require.Error(t, verifyModelChecksum(path, fmt.Sprintf("%x", expected)))
}

func TestInferModelName_RecognizesLarge(t *testing.T) {
	require.Equal(t, "large", inferModelName("/tmp/models/ggml-large.bin"))
}

func TestModelCatalog_ContainsWeightsAndWarnings(t *testing.T) {
	large, ok := ModelInfoFor("large")
	require.True(t, ok)
	require.Equal(t, "ggml-large-v3.bin", large.FileName)
	require.Equal(t, "3.1 GB", large.DisplaySize)
	require.NotEmpty(t, large.ResourceWarning)
}
