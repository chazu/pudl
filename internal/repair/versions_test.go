package repair

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

// TestVersionsRepair renumbers a resource whose observed entries were all
// recorded as version 1 (pudl-42w), in import order, and leaves a resource
// with a unique history alone. It is opt-in and idempotent.
func TestVersionsRepair(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	v := func(n int) *int { return &n }
	for _, e := range []database.CatalogEntry{
		{ID: "late", ResourceID: strptr("R"), Version: v(1), ImportTimestamp: time.Unix(3000, 0)},
		{ID: "early", ResourceID: strptr("R"), Version: v(1), ImportTimestamp: time.Unix(1000, 0)},
		{ID: "middle", ResourceID: strptr("R"), Version: v(1), ImportTimestamp: time.Unix(2000, 0)},
		{ID: "s1", ResourceID: strptr("S"), Version: v(1), ImportTimestamp: time.Unix(1000, 0)},
		{ID: "s2", ResourceID: strptr("S"), Version: v(2), ImportTimestamp: time.Unix(500, 0)},
	} {
		e.Schema, e.Format, e.StoredPath = "pudl/core.#Item", "json", "/dev/null"
		require.NoError(t, db.AddEntry(e))
	}
	env := &Env{DB: db}
	assert.True(t, versions.OptIn)

	preview := Run(context.Background(), versions, env, true, true)
	require.Equal(t, StatusNeeded, preview.Status, "%+v", preview)
	assert.Equal(t, 2, preview.Pending, "early stays version 1; middle and late move")
	got, err := db.GetEntry("late")
	require.NoError(t, err)
	assert.Equal(t, 1, *got.Version, "dry run wrote")

	applied := Run(context.Background(), versions, env, true, false)
	require.Equal(t, StatusApplied, applied.Status, "%+v", applied)
	for id, want := range map[string]int{"early": 1, "middle": 2, "late": 3, "s1": 1, "s2": 2} {
		got, err := db.GetEntry(id)
		require.NoError(t, err)
		assert.Equal(t, want, *got.Version, id)
	}

	again := Run(context.Background(), versions, env, false, false)
	assert.Equal(t, StatusOK, again.Status, "%+v", again)
}
