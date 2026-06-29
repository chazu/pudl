package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
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
		{"_schema": "pudl/linux.#Package", "name": "htop", "state": "present"},    // missing
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "absent"},   // changed
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
	_, err = mubridge.IngestObserveResults(db, strings.NewReader(canned), "pudl-run", dataDir, nil)
	require.NoError(t, err)

	desired := []map[string]any{
		{"_schema": "pudl/linux.#Package", "name": "podman", "state": "present"}, // satisfied
		{"_schema": "pudl/linux.#Package", "name": "htop", "state": "present"},    // missing
		{"_schema": "pudl/linux.#Package", "name": "restic", "state": "absent"},   // changed
	}
	res, err := runInventoryDrift(db, "pudl-run", desired, nil)
	require.NoError(t, err)

	assert.False(t, res.Clean)
	require.Len(t, res.Drifted, 2, "htop missing + restic changed; podman satisfied")
}
