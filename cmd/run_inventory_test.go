package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/errors"
	"github.com/chazu/pudl/internal/mubridge"
)

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

	changed := res.Drifted[1]
	assert.Equal(t, "changed", changed.Reason)
	assert.Equal(t, []acute.FieldDiff{{Path: "state", Expected: "absent", Observed: "present"}}, changed.Fields)
	require.NotNil(t, changed.ObservedAt, "the compared record's import time is the observation time")
}

// A model whose desired records carry no identity used to compare nothing and
// report clean.
func TestRunInventoryDrift_UnidentifiableDesiredIsNotClean(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewCatalogDB(filepath.Join(dir, "db"))
	require.NoError(t, err)
	defer db.Close()

	canned := `[{"target":"//host:odroid","current":{"records":[
		{"_schema":"pudl/linux.#Package","state":"present"}
	]}}]`
	_, err = mubridge.IngestObserve(db, mubridge.ObserveIngest{Reader: strings.NewReader(canned), Origin: "pudl-run", DataDir: filepath.Join(dir, "data"), Graph: nil})
	require.NoError(t, err)

	res, err := runInventoryDrift(db, "pudl-run", []map[string]any{
		{"_schema": "pudl/linux.#Package", "state": "present"},
	}, nil)
	require.NoError(t, err)
	assert.False(t, res.Clean)
	require.Len(t, res.Drifted, 1)
	assert.Equal(t, "unidentifiable", res.Drifted[0].Reason)
}

// The human run report renders drift from the same struct the JSON report
// serializes, so both carry the same findings.
func TestRunReportDriftHumanMatchesJSON(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	report := &RunReport{
		RunID: "run-1", Model: "m", Mode: "observe-only", OK: true,
		Drift: &ModelDriftResult{
			Drifted: acute.InventorySetDiff(
				[]map[string]any{{"_schema": "s", "name": "a", "port": int64(1), "state": "on"}},
				[]acute.ObservedRecord{{Data: map[string]any{"_schema": "s", "name": "a", "port": "1", "state": "off"}, ObservedAt: &at}},
				nil,
			),
			SnapshotID: "snap-1",
			ObservedAt: &at,
		},
	}
	human, err := report.render(false)
	require.NoError(t, err)
	machine, err := report.render(true)
	require.NoError(t, err)

	var decoded struct {
		Drift struct {
			Drifted []struct {
				Resource string            `json:"resource"`
				Reason   string            `json:"reason"`
				Diff     string            `json:"diff"`
				Fields   []acute.FieldDiff `json:"fields"`
			} `json:"drifted"`
			Verified   bool   `json:"verified"`
			SnapshotID string `json:"snapshot_id"`
			ObservedAt string `json:"observed_at"`
		} `json:"drift"`
	}
	require.NoError(t, json.Unmarshal([]byte(machine), &decoded))
	require.Len(t, decoded.Drift.Drifted, 1)
	finding := decoded.Drift.Drifted[0]
	require.Len(t, finding.Fields, 2)
	assert.Contains(t, human, finding.Resource+" ("+finding.Reason+"): "+finding.Diff)
	for _, f := range finding.Fields {
		assert.Contains(t, human, f.Detail())
	}
	assert.Contains(t, human, "- snapshot_id: "+decoded.Drift.SnapshotID)
	assert.Contains(t, human, "- observed_at: "+decoded.Drift.ObservedAt)
	assert.Contains(t, human, "- verified: false")
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
