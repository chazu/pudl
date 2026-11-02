package observe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/artifacts"
	"github.com/stretchr/testify/require"
)

// Observed records and the snapshot are 0600 files under 0700 directories.
func TestIngestObserveWritesOwnerOnlyData(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	_, err := IngestObserve(db, ObserveIngest{Reader: strings.NewReader(snapshotTestInput), DataDir: dataDir, SnapshotID: "snap_private"})
	require.NoError(t, err)
	records, err := db.SnapshotRecordEntries("snap_private")
	require.NoError(t, err)
	require.NotEmpty(t, records)
	snapshot, err := db.GetEntry("snap_private")
	require.NoError(t, err)

	for _, path := range []string{snapshot.StoredPath, records[0].StoredPath} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, artifacts.PrivateFileMode, info.Mode().Perm(), path)
	}
	loose, err := artifacts.FindLoosePaths(dataDir)
	require.NoError(t, err)
	require.Empty(t, loose)
	info, err := os.Stat(filepath.Join(dataDir, "raw"))
	require.NoError(t, err)
	require.Equal(t, artifacts.PrivateDirMode, info.Mode().Perm())
}
