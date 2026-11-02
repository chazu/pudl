package inventory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func zeroPolicy() (Policy, error) { return Policy{}, nil }

// Absence from a partial snapshot is not a removal, and a value change is.
func TestCompareSnapshotsKeepsPartialAbsenceUnproven(t *testing.T) {
	db, dataDir := inventoryFixture(t)
	ingestSnapshot(t, db, dataDir, "snap_a", "hostinv", `[{"target":"//host:box","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"},
		{"_schema":"pudl/linux.#Package","name":"restic","state":"present"}
	]}}]`)
	ingestSnapshot(t, db, dataDir, "snap_b", "hostinv", `[{"target":"//host:box","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"absent"},
		{"_schema":"pudl/linux.#Package","name":"htop","state":"present"}
	]}}]`)

	cmp, err := CompareSnapshots(context.Background(), db, "snap_a", "snap_b", zeroPolicy)
	require.NoError(t, err)
	got := map[string]string{}
	for _, change := range cmp.Changes {
		got[change.Resource] = change.Change
	}
	require.Len(t, got, 3)
	assert.Equal(t, "changed", got["pudl/linux.#Package/podman"])
	assert.Equal(t, "not-observed", got["pudl/linux.#Package/restic"])
	assert.Equal(t, "newly-observed", got["pudl/linux.#Package/htop"])
	assert.Equal(t, map[string][]string{"pudl/linux.#Package": {"name"}}, cmp.IdentityFallbacks)
}

func TestCompareSnapshotsRefusesDifferentOwners(t *testing.T) {
	db, dataDir := inventoryFixture(t)
	ingestSnapshot(t, db, dataDir, "snap_a", "one", `[{"target":"//host:box","current":{"records":[{"name":"x"}]}}]`)
	ingestSnapshot(t, db, dataDir, "snap_b", "two", `[{"target":"//host:box","current":{"records":[{"name":"x"}]}}]`)
	_, err := CompareSnapshots(context.Background(), db, "snap_a", "snap_b", zeroPolicy)
	require.ErrorContains(t, err, "different observation scopes or owners")
	_, err = CompareSnapshots(context.Background(), db, "snap_a", "snap_missing", zeroPolicy)
	require.ErrorContains(t, err, "two retained observation snapshots")
}
