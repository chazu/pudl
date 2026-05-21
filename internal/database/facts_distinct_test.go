package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDistinctRelations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Empty DB returns empty slice
	relations, err := db.GetDistinctRelations()
	require.NoError(t, err)
	assert.Empty(t, relations)

	// Add facts with different relations
	_, err = db.AddFact(Fact{Relation: "observation", Args: `{"kind":"bug"}`, Source: "test"})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "depends", Args: `{"from":"a","to":"b"}`, Source: "test"})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "observation", Args: `{"kind":"risk"}`, Source: "test"})
	require.NoError(t, err)

	relations, err = db.GetDistinctRelations()
	require.NoError(t, err)
	assert.Equal(t, []string{"depends", "observation"}, relations)
}

func TestGetDistinctRelationsExcludesRetracted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	f, err := db.AddFact(Fact{Relation: "ephemeral", Args: `{"x":1}`, Source: "test"})
	require.NoError(t, err)

	_, err = db.AddFact(Fact{Relation: "persistent", Args: `{"y":2}`, Source: "test"})
	require.NoError(t, err)

	// Retract the ephemeral fact
	err = db.RetractFact(f.ID)
	require.NoError(t, err)

	relations, err := db.GetDistinctRelations()
	require.NoError(t, err)
	assert.Equal(t, []string{"persistent"}, relations)
}

func TestGetDistinctSources(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Empty DB returns empty slice
	sources, err := db.GetDistinctSources()
	require.NoError(t, err)
	assert.Empty(t, sources)

	// Add facts with different sources
	_, err = db.AddFact(Fact{Relation: "observation", Args: `{"kind":"bug"}`, Source: "claude-code"})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "observation", Args: `{"kind":"risk"}`, Source: "human"})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "depends", Args: `{"from":"a","to":"b"}`, Source: "claude-code"})
	require.NoError(t, err)

	sources, err = db.GetDistinctSources()
	require.NoError(t, err)
	assert.Equal(t, []string{"claude-code", "human"}, sources)
}

func TestGetDistinctSourcesExcludesEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Add a fact with empty source
	_, err := db.AddFact(Fact{Relation: "observation", Args: `{"kind":"bug"}`, Source: ""})
	require.NoError(t, err)
	_, err = db.AddFact(Fact{Relation: "observation", Args: `{"kind":"risk"}`, Source: "agent"})
	require.NoError(t, err)

	sources, err := db.GetDistinctSources()
	require.NoError(t, err)
	assert.Equal(t, []string{"agent"}, sources)
}
