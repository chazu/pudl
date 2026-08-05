package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenCatalogDBReadOnlyReadsExistingCatalogAndRejectsWrites(t *testing.T) {
	root := t.TempDir()
	db, err := NewCatalogDB(root)
	require.NoError(t, err)
	require.NoError(t, db.StartRun("run_1", "model", "observe-only"))
	require.NoError(t, db.Close())

	readOnly, err := OpenCatalogDBReadOnly(root)
	require.NoError(t, err)
	defer readOnly.Close()
	runs, err := readOnly.UnfinishedRuns("model")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	_, err = readOnly.DB().Exec(`DELETE FROM runs`)
	require.Error(t, err)
}

func TestOpenCatalogDBReadOnlyDoesNotCreateMissingCatalog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	_, err := OpenCatalogDBReadOnly(root)
	require.Error(t, err)
	_, statErr := os.Stat(root)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
