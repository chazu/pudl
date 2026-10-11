package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetireGuessedDependenciesPreservesOtherSourcesAndHistory(t *testing.T) {
	dir := t.TempDir()
	db, err := NewCatalogDB(dir)
	require.NoError(t, err)
	for _, source := range []string{"derived:consumer", "model:consumer", "binding:consumer", "user", "derived:other"} {
		_, err := db.AddFact(Fact{Relation: "model_depends_on", Source: source, Args: `{"from":"consumer","to":"producer"}`})
		require.NoError(t, err)
	}
	_, err = db.AddFact(Fact{Relation: "other", Source: "derived:consumer", Args: `{"from":"consumer","to":"producer"}`})
	require.NoError(t, err)
	_, err = db.db.Exec("DELETE FROM schema_migrations WHERE version = 24")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	db, err = NewCatalogDB(dir)
	require.NoError(t, err)
	defer db.Close()
	facts, err := db.QueryFacts(FactFilter{Relation: "model_depends_on"})
	require.NoError(t, err)
	require.Len(t, facts, 4)
	for _, f := range facts {
		require.NotEqual(t, "derived:consumer", f.Source)
	}
	history, err := db.FactHistory("model_depends_on")
	require.NoError(t, err)
	require.Len(t, history, 5)
	other, err := db.QueryFacts(FactFilter{Relation: "other"})
	require.NoError(t, err)
	require.Len(t, other, 1)
}
