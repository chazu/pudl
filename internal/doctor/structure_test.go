package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// pudl-bpy: the install records of pudl example install (.examples) and pudl
// pack install (.packs) are pudl's own bookkeeping, not unexpected entries.
func TestDirectoryStructureAcceptsInstallRecords(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"data/raw", "data/metadata", "data/sqlite", "schema/cue.mod", "schema/pudl/core", ".examples", ".packs"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
	}

	result := CheckDirectoryStructureAt(root)
	require.Equal(t, "ok", result.Status, "%s: %s", result.Message, result.Details)

	require.NoError(t, os.Mkdir(filepath.Join(root, "stray"), 0o755))
	result = CheckDirectoryStructureAt(root)
	require.Equal(t, "warning", result.Status)
	require.Contains(t, result.Details, "[stray]")
}
