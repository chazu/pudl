package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogEntryColumnsMatchTheView(t *testing.T) {
	dir, err := os.MkdirTemp("", "view-cols")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	db, err := NewCatalogDB(dir)
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.db.Query(`SELECT name FROM pragma_table_info(?)`, CatalogEntryView)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got = append(got, name)
	}
	require.ElementsMatch(t, CatalogEntryColumns, got)
}
