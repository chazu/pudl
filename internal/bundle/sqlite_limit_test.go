package bundle

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOnlineSnapshotBudgetBeforeDestinationCreation(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "source.db"))
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec("CREATE TABLE evidence (body BLOB); INSERT INTO evidence VALUES (zeroblob(1048576))")
	require.NoError(t, err)
	var pages, pageSize int64
	require.NoError(t, db.QueryRow("PRAGMA page_count").Scan(&pages))
	require.NoError(t, db.QueryRow("PRAGMA page_size").Scan(&pageSize))
	limit := pages * pageSize
	destination := filepath.Join(t.TempDir(), "snapshot.db")
	err = onlineSnapshot(context.Background(), db, destination, limit-1)
	require.ErrorContains(t, err, "catalog exceeds bundle byte limit")
	_, err = os.Stat(destination)
	require.True(t, os.IsNotExist(err), "preflight must not create an oversized snapshot")
	require.NoError(t, onlineSnapshot(context.Background(), db, destination, limit))
	info, err := os.Stat(destination)
	require.NoError(t, err)
	require.Equal(t, limit, info.Size())
	copied, err := sql.Open("sqlite", destination)
	require.NoError(t, err)
	defer copied.Close()
	var length int
	require.NoError(t, copied.QueryRow("SELECT length(body) FROM evidence").Scan(&length))
	require.Equal(t, 1048576, length)
}
