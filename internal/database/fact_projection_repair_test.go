package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenRepairsLegacyFactProjections(t *testing.T) {
	dir := t.TempDir()
	db, err := NewCatalogDB(dir)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	live, err := db.AddFact(Fact{Relation: "evidence", Args: `{"text":"original"}`, ValidStart: 1})
	require.NoError(t, err)
	ended, err := db.AddFact(Fact{Relation: "evidence", Args: `{"text":"retracted"}`, ValidStart: 1})
	require.NoError(t, err)
	require.NoError(t, db.RetractFact(ended.ID))
	history, err := db.FactHistory("evidence")
	require.NoError(t, err)

	// Recreate the old replay bug, including its stale search index. The repair
	// also restores a live fact's payload from the authoritative historical row.
	require.NoError(t, insertCurrentFact(db.db, ended))
	stale := live
	stale.Args = `{"text":"overwritten"}`
	require.NoError(t, insertCurrentFact(db.db, stale))
	_, err = db.db.Exec(`DELETE FROM schema_migrations WHERE version = 17`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	for range 2 {
		db, err = NewCatalogDB(dir)
		require.NoError(t, err)
		current, err := db.QueryCurrentFacts("evidence")
		require.NoError(t, err)
		require.Len(t, current, 1)
		require.Equal(t, live.Args, current[0].Args)
		for _, term := range []string{"retracted", "overwritten"} {
			found, err := db.SearchCurrentFacts(term, "evidence", 0)
			require.NoError(t, err)
			require.Empty(t, found)
		}
		found, err := db.SearchCurrentFacts("original", "evidence", 0)
		require.NoError(t, err)
		require.Len(t, found, 1)
		after, err := db.FactHistory("evidence")
		require.NoError(t, err)
		require.Equal(t, history, after, "repair preserves historical facts and IDs")
		require.NoError(t, db.Close())
	}
}
