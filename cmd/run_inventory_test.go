package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/mubridge"
)

// pure set-diff logic — synthetic records, no DB.
func TestInventorySetDiff(t *testing.T) {
	observed := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"},
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "present"},
	}
	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"}, // satisfied
		{"_schema": "pudl/linux.#Package", "name": "htop", "state": "present"},   // missing
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "absent"},  // changed
	}
	drift := inventorySetDiff(desired, observed, nil) // nil resolver -> name|path|id fallback
	require.Len(t, drift, 2)

	byReason := map[string]ResourceDrift{}
	for _, d := range drift {
		byReason[d.Reason] = d
	}
	assert.Contains(t, byReason["missing"].Resource, "htop")
	assert.Contains(t, byReason["changed"].Resource, "restic")
	assert.Contains(t, byReason["changed"].Diff, "state")
}

// schema-driven identity: match on declared identity_fields (composite), not the
// name|path|id fallback (these records carry none of those).
func TestInventorySetDiff_SchemaDrivenIdentity(t *testing.T) {
	identity := func(schema string) []string {
		if schema == "pudl/artifact.#ImageRef" {
			return []string{"source", "tag"}
		}
		return nil
	}
	observed := []map[string]any{
		{"_schema": "pudl/artifact.#ImageRef", "source": "ghcr.io/o/i", "tag": "v1", "digest": "sha256:aaa"},
	}
	desired := []map[string]any{
		// same (source,tag) identity, differing non-identity field -> changed
		{"_schema": "pudl/artifact.#ImageRef", "source": "ghcr.io/o/i", "tag": "v1", "digest": "sha256:bbb"},
		// different tag -> different identity -> missing
		{"_schema": "pudl/artifact.#ImageRef", "source": "ghcr.io/o/i", "tag": "v2", "digest": "sha256:aaa"},
	}
	drift := inventorySetDiff(desired, observed, identity)
	require.Len(t, drift, 2)

	byReason := map[string]ResourceDrift{}
	for _, d := range drift {
		byReason[d.Reason] = d
	}
	assert.Contains(t, byReason["changed"].Diff, "digest")
	assert.Contains(t, byReason["missing"].Resource, "v2")
}

func TestInventorySetDiff_AllSatisfied(t *testing.T) {
	recs := []map[string]any{{"_schema": "s", "name": "a", "x": "1"}}
	drift := inventorySetDiff(recs, recs, nil)
	assert.Empty(t, drift)
}

func TestInventorySetDiff_ExtrasIgnored(t *testing.T) {
	// observed has an extra not in desired -> not drift (ensure-present).
	observed := []map[string]any{
		{"_schema": "s", "name": "a"}, {"_schema": "s", "name": "extra"},
	}
	desired := []map[string]any{{"_schema": "s", "name": "a"}}
	assert.Empty(t, inventorySetDiff(desired, observed, nil))
}

// end-to-end against a real catalog seeded with CANNED host-style records (the
// mock — exactly what an inventory observer like `host` emits). No SSH/docker.
func TestRunInventoryDrift_RealCatalog(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(filepath.Join(dir, "db"))
	require.NoError(t, err)
	defer db.Close()

	canned := `[{"target":"//host:odroid","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"},
		{"_schema":"pudl/linux.#Package","name":"restic","state":"present"}
	]}}]`
	dataDir := filepath.Join(dir, "data")
	_, err = mubridge.IngestObserve(db, mubridge.ObserveIngest{Reader: strings.NewReader(canned), Origin: "pudl-run", DataDir: dataDir, Graph: nil})
	require.NoError(t, err)

	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"}, // satisfied
		{"_schema": "pudl/linux.#Package", "name": "htop", "state": "present"},   // missing
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "absent"},  // changed
	}
	res, err := runInventoryDrift(db, "pudl-run", desired, nil)
	require.NoError(t, err)

	assert.False(t, res.Clean)
	require.Len(t, res.Drifted, 2, "htop missing + restic changed; podman satisfied")
	assert.False(t, res.Verified, "runInventoryDrift cannot know whether its scope is fresh")
}

// A failed snapshot lookup used to degrade into an origin filter regardless of
// why it failed. For a genuine DB error that filter matches nothing, so the
// set-diff sees no observed records and calls every desired resource `missing` —
// under --converge, re-applying the whole model off a transient fault.
func TestObserveScopeFilter(t *testing.T) {
	t.Run("found scope filters by collection", func(t *testing.T) {
		collectionID, origin, err := observeScopeFilter("snap-1", nil)
		require.NoError(t, err)
		assert.Equal(t, "snap-1", collectionID)
		assert.Empty(t, origin)
	})

	t.Run("not-found scope falls back to origin", func(t *testing.T) {
		notFound := errors.WrapError(errors.ErrCodeNotFound, "Collection not found: pudl-run", nil)
		collectionID, origin, err := observeScopeFilter("pudl-run", notFound)
		require.NoError(t, err)
		assert.Empty(t, collectionID)
		assert.Equal(t, "pudl-run", origin, "the compatibility path stays open for origin-scoped callers")
	})

	t.Run("database error is fatal, not an empty observed set", func(t *testing.T) {
		dbErr := errors.WrapError(errors.ErrCodeDatabaseError, "Failed to retrieve collection", fmt.Errorf("disk I/O error"))
		collectionID, origin, err := observeScopeFilter("snap-1", dbErr)
		require.Error(t, err, "a DB fault must not silently become 'nothing observed'")
		assert.Contains(t, err.Error(), "resolve observe scope")
		assert.Empty(t, collectionID)
		assert.Empty(t, origin)
	})
}

// An empty scope previously queried every observation in the catalog, so a
// desired record could be satisfied by an unrelated model's records.
func TestRunInventoryDriftRequiresAScope(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(filepath.Join(dir, "db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = runInventoryDrift(db, "  ", []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"},
	}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a catalog scope")
}

// Records ingested under one origin must not satisfy another scope's desired
// state: that is the false-clean this scoping exists to prevent.
func TestRunInventoryDriftDoesNotMatchAcrossScopes(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(filepath.Join(dir, "db"))
	require.NoError(t, err)
	defer db.Close()

	dataDir := filepath.Join(dir, "data")
	other := `[{"target":"//host:other","current":{"records":[
		{"_schema":"pudl/linux.#Package","name":"podman","state":"present"}
	]}}]`
	_, err = mubridge.IngestObserve(db, mubridge.ObserveIngest{Reader: strings.NewReader(other), Origin: "other-model", DataDir: dataDir, Graph: nil})
	require.NoError(t, err)

	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"},
	}

	// Scoped to a model with no records of its own: the other model's matching
	// record must not satisfy it.
	res, err := runInventoryDrift(db, "this-model", desired, nil)
	require.NoError(t, err)
	assert.False(t, res.Clean)
	require.Len(t, res.Drifted, 1)
	assert.Equal(t, "missing", res.Drifted[0].Reason)

	// Scoped to the origin that does hold the record, it is satisfied.
	res, err = runInventoryDrift(db, "other-model", desired, nil)
	require.NoError(t, err)
	assert.True(t, res.Clean)
}
