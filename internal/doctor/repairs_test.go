package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/repair"
)

func selectedRepairs(t *testing.T, names ...string) []repair.Repair {
	t.Helper()
	selected, err := repair.Select(names)
	require.NoError(t, err)
	return selected
}

func catalogPath(pudlDir string) string {
	return filepath.Join(pudlDir, "data", "sqlite", "catalog.db")
}

// Without a catalog every repair is ok, and none is created.
func TestRunRepairsWithoutACatalog(t *testing.T) {
	dir := t.TempDir()
	results := RunRepairs(context.Background(), RepairRun{Selected: selectedRepairs(t, "schema-names"), PudlDir: dir, Fix: true, HealthOK: true})
	require.Len(t, results, 1)
	assert.Equal(t, repair.StatusOK, results[0].Status)
	assert.Equal(t, "no catalog", results[0].Message)
	assert.NoFileExists(t, catalogPath(dir))
}

func TestRunRepairsRefusesToFixAnUnhealthyWorkspace(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	results := RunRepairs(context.Background(), RepairRun{Selected: selectedRepairs(t, "schema-names", "identity"), PudlDir: dir, Fix: true, HealthOK: false})
	require.Len(t, results, 2)
	for _, result := range results {
		assert.Equal(t, repair.StatusSkipped, result.Status)
	}
}

// A repair environment that cannot be built is every repair's error, not a
// failure of the run.
func TestRunRepairsReportsAnEnvironmentFailurePerRepair(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	results := RunRepairs(context.Background(), RepairRun{
		Selected: selectedRepairs(t, "schema-names"), PudlDir: dir, HealthOK: true,
		Env: func(*database.CatalogDB) (*repair.Env, error) { return nil, errors.New("no schemas") },
	})
	require.Len(t, results, 1)
	assert.Equal(t, repair.StatusError, results[0].Status)
	assert.Equal(t, "no schemas", results[0].Message)
}

func TestCheckCatalogEntriesWithoutACatalog(t *testing.T) {
	dir := t.TempDir()
	entries, secrets, err := CheckCatalogEntries(context.Background(), CatalogScan{DataPath: filepath.Join(dir, "data")})
	require.NoError(t, err)
	assert.Empty(t, entries)
	require.NotNil(t, secrets)
	assert.True(t, secrets.FactsScanned)
	_, statErr := os.Stat(catalogPath(dir))
	assert.True(t, os.IsNotExist(statErr), "checking must not create a catalog")
}

func TestDriftEntryIDs(t *testing.T) {
	ids := DriftEntryIDs([]CatalogDiagnostic{{ID: "a", Status: "ok"}, {ID: "b", Status: "mismatch"}, {ID: "c", Status: "invalid"}})
	assert.Equal(t, []string{"b"}, ids)
	assert.Nil(t, DriftEntryIDs(nil))
}
