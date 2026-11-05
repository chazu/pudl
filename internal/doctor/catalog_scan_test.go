package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCatalogEntriesWithoutACatalog(t *testing.T) {
	dir := t.TempDir()
	entries, secrets, err := CheckCatalogEntries(context.Background(), CatalogScan{PudlDir: dir})
	require.NoError(t, err)
	assert.Empty(t, entries)
	require.NotNil(t, secrets)
	assert.True(t, secrets.FactsScanned)
	_, statErr := os.Stat(filepath.Join(dir, "data", "sqlite", "catalog.db"))
	assert.True(t, os.IsNotExist(statErr), "checking must not create a catalog")
}

func TestDriftEntryIDs(t *testing.T) {
	ids := DriftEntryIDs([]CatalogDiagnostic{{ID: "a", Status: "ok"}, {ID: "b", Status: "mismatch"}, {ID: "c", Status: "invalid"}})
	assert.Equal(t, []string{"b"}, ids)
	assert.Nil(t, DriftEntryIDs(nil))
}
