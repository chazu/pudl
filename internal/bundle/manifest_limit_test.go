package bundle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArchiveWriterRefusesUnreadableManifestBeforeOutputCreation(t *testing.T) {
	outputDir := t.TempDir()
	destination := filepath.Join(outputDir, "backup.tar.zst")
	require.NoError(t, os.WriteFile(destination, []byte("previous backup"), 0600))
	manifest := Manifest{Version: Version, OriginalRoot: t.TempDir(), Notes: []string{strings.Repeat("x", maxManifestBytes)}}
	err := writeArchive(context.Background(), t.TempDir(), destination, manifest)
	require.ErrorContains(t, err, "bundle manifest exceeds size limit")
	body, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "previous backup", string(body))
	files, err := os.ReadDir(outputDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
}
