package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *CatalogDB {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "pudl-current-facts-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	db, err := NewCatalogDB(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCurrentFacts_AddPopulatesCurrent(t *testing.T) {
	db := newTestDB(t)

	f, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"target":"svc-a","kind":"healthy"}`,
		Source:   "test",
	})
	require.NoError(t, err)

	facts, err := db.QueryCurrentFacts("observation")
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, f.ID, facts[0].ID)
	assert.Equal(t, "observation", facts[0].Relation)
}

func TestCurrentFacts_RetractRemovesFromCurrent(t *testing.T) {
	db := newTestDB(t)

	f, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"target":"svc-a","kind":"healthy"}`,
		Source:   "test",
	})
	require.NoError(t, err)

	err = db.RetractFact(f.ID)
	require.NoError(t, err)

	facts, err := db.QueryCurrentFacts("observation")
	require.NoError(t, err)
	assert.Empty(t, facts)
}

func TestCurrentFacts_InvalidateRemovesFromCurrent(t *testing.T) {
	db := newTestDB(t)

	f, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"target":"svc-a","kind":"healthy"}`,
		Source:   "test",
	})
	require.NoError(t, err)

	err = db.InvalidateFact(f.ID)
	require.NoError(t, err)

	facts, err := db.QueryCurrentFacts("observation")
	require.NoError(t, err)
	assert.Empty(t, facts)
}

func TestCurrentFacts_NonCurrentFactNotInCurrent(t *testing.T) {
	db := newTestDB(t)

	txEnd := int64(1000)
	_, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"target":"svc-a","kind":"healthy"}`,
		Source:   "test",
		TxEnd:    &txEnd,
	})
	require.NoError(t, err)

	facts, err := db.QueryCurrentFacts("observation")
	require.NoError(t, err)
	assert.Empty(t, facts)
}

func TestCurrentFacts_FilteredQuery(t *testing.T) {
	db := newTestDB(t)

	_, err := db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"target":"svc-a","kind":"healthy"}`,
		Source:   "test",
	})
	require.NoError(t, err)

	_, err = db.AddFact(Fact{
		Relation: "observation",
		Args:     `{"target":"svc-b","kind":"unhealthy"}`,
		Source:   "test",
	})
	require.NoError(t, err)

	facts, err := db.QueryCurrentFactsFiltered("observation", map[string]interface{}{
		"kind": "healthy",
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Contains(t, facts[0].Args, "svc-a")
}

func TestCurrentFacts_ListRelations(t *testing.T) {
	db := newTestDB(t)

	_, err := db.AddFact(Fact{Relation: "alpha", Args: `{"x":1}`, Source: "test"})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "beta", Args: `{"x":2}`, Source: "test"})
	require.NoError(t, err)

	rels, err := db.ListCurrentRelations()
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, rels)
}

func TestCurrentFacts_Count(t *testing.T) {
	db := newTestDB(t)

	_, err := db.AddFact(Fact{Relation: "obs", Args: `{"x":1}`, Source: "test"})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "obs", Args: `{"x":2}`, Source: "test"})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "other", Args: `{"x":3}`, Source: "test"})
	require.NoError(t, err)

	total, err := db.CountCurrentFacts("")
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	obsCount, err := db.CountCurrentFacts("obs")
	require.NoError(t, err)
	assert.Equal(t, 2, obsCount)
}

func TestCurrentFacts_BackfillOnReopen(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pudl-backfill-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	db, err := NewCatalogDB(tmpDir)
	require.NoError(t, err)

	_, err = db.AddFact(Fact{Relation: "obs", Args: `{"x":1}`, Source: "test"})
	require.NoError(t, err)
	db.Close()

	// Simulate pre-migration state: drop current_facts
	db2, err := NewCatalogDB(tmpDir)
	require.NoError(t, err)

	// current_facts should have been backfilled on open
	facts, err := db2.QueryCurrentFacts("obs")
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	db2.Close()
}
