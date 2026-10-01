package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentMemoryRetirementPreservesFactsHistoryAndSearch(t *testing.T) {
	root := t.TempDir()
	old, err := NewCatalogDB(root)
	require.NoError(t, err)
	current, err := old.AddFact(Fact{Relation: "observation", Args: `{"description":"retained telemetry","status":"promoted","worth":0.8}`, Source: "legacy-agent"})
	require.NoError(t, err)
	withdrawn, err := old.AddFact(Fact{Relation: "feedback", Args: `{"target":"old","verdict":"harmful"}`, Source: "legacy-agent"})
	require.NoError(t, err)
	require.NoError(t, old.RetractFact(withdrawn.ID))
	historyBefore, err := old.GetFact(withdrawn.ID)
	require.NoError(t, err)
	_, err = old.db.Exec("DELETE FROM schema_migrations WHERE version = 18")
	require.NoError(t, err)
	_, err = old.db.Exec("CREATE VIEW fact_scored_edb AS SELECT id, relation FROM current_facts")
	require.NoError(t, err)
	require.NoError(t, old.Close())
	migrated, err := NewCatalogDB(root)
	require.NoError(t, err)
	defer migrated.Close()
	var views int
	require.NoError(t, migrated.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='view' AND name='fact_scored_edb'").Scan(&views))
	require.Zero(t, views)
	after, err := migrated.GetFact(current.ID)
	require.NoError(t, err)
	require.Equal(t, current, *after)
	historical, err := migrated.GetFact(withdrawn.ID)
	require.NoError(t, err)
	require.Equal(t, historyBefore, historical)
	matches, err := migrated.SearchCurrentFacts("telemetry", "observation", 10)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, current.ID, matches[0].ID)
	require.False(t, IsReservedRelation("fact_scored"))
	// The old relation name can be used as ordinary data without restoring policy.
	_, err = migrated.AddFact(Fact{Relation: "fact_scored", Args: `{"value":1}`})
	require.NoError(t, err)
	require.Contains(t, migrationVersions(t, migrated), 18)
}
