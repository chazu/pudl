package repair

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

func selectedRepairs(t *testing.T, names ...string) []Repair {
	t.Helper()
	selected, err := Select(names)
	require.NoError(t, err)
	return selected
}

func catalogPath(pudlDir string) string {
	return filepath.Join(pudlDir, "data", "sqlite", "catalog.db")
}

// Without a catalog every repair is ok, and none is created.
func TestRunPassWithoutACatalog(t *testing.T) {
	dir := t.TempDir()
	results := RunPass(context.Background(), Pass{Selected: selectedRepairs(t, "schema-names"), PudlDir: dir, Fix: true, HealthOK: true})
	require.Len(t, results, 1)
	assert.Equal(t, StatusOK, results[0].Status)
	assert.Equal(t, "no catalog", results[0].Message)
	assert.NoFileExists(t, catalogPath(dir))
}

func TestRunPassRefusesToFixAnUnhealthyWorkspace(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	results := RunPass(context.Background(), Pass{Selected: selectedRepairs(t, "schema-names", "identity"), PudlDir: dir, Fix: true, HealthOK: false})
	require.Len(t, results, 2)
	for _, result := range results {
		assert.Equal(t, StatusSkipped, result.Status)
	}
}

// A repair environment that cannot be built is every repair's error, not a
// failure of the run.
func TestRunPassReportsAnEnvironmentFailurePerRepair(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	results := RunPass(context.Background(), Pass{
		Selected: selectedRepairs(t, "schema-names"), PudlDir: dir, HealthOK: true,
		Env: func(*database.CatalogDB) (*Env, error) { return nil, errors.New("no schemas") },
	})
	require.Len(t, results, 1)
	assert.Equal(t, StatusError, results[0].Status)
	assert.Equal(t, "no schemas", results[0].Message)
}

// --fix applies an opt-in repair only when --only names it.
func TestRunPassSkipsAnUnnamedOptInRepair(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(dir)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	results := RunPass(context.Background(), Pass{
		Selected: selectedRepairs(t, "sensitive-scrub"), PudlDir: dir, Fix: true, HealthOK: true,
		Env: func(db *database.CatalogDB) (*Env, error) { return &Env{DB: db}, nil },
	})
	require.Len(t, results, 1)
	assert.Equal(t, StatusSkipped, results[0].Status)
	assert.Equal(t, "opt-in: name it with --only sensitive-scrub", results[0].Message)
}
