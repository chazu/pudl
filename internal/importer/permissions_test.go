package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/stretchr/testify/require"
)

// Imported payloads, collection items and metadata are 0600 files under 0700
// directories, as is the catalog the import creates.
func TestImportWritesOwnerOnlyData(t *testing.T) {
	imp, source, dataDir := collectionFixture(t, "records.ndjson", ndjsonRecords(3))
	collection, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	single := filepath.Join(filepath.Dir(source), "single.json")
	require.NoError(t, os.WriteFile(single, []byte(`{"name":"one"}`), 0o644))
	plain, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: single})
	require.NoError(t, err)
	items, err := imp.catalogDB.GetCollectionItems(collection.ID)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	for _, path := range []string{collection.StoredPath, collection.MetadataPath, items[0].StoredPath, plain.StoredPath, plain.MetadataPath} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, artifacts.PrivateFileMode, info.Mode().Perm(), path)
		dir, err := os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		require.Equal(t, artifacts.PrivateDirMode, dir.Mode().Perm(), filepath.Dir(path))
	}
	for _, sub := range []string{"raw", "metadata", "sqlite", "tmp"} {
		loose, err := artifacts.FindLoosePaths(filepath.Join(dataDir, sub))
		require.NoError(t, err)
		require.Empty(t, loose, sub)
	}
}
