package scrub

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/doctor"
)

// Unstructured artifacts quoting a collected value are found by value search:
// an observe snapshot's summary (whose pre-allocated ID is kept) and another
// entry's .meta file.
func TestScrubValueSearchesSnapshotsAndMetadata(t *testing.T) {
	f := newFixture(t)
	doc := f.importFile("solo.json", `{"name":"solo","password":"`+secretC+`"}`)
	f.declare()
	s, db := f.scrubber()

	// The flagged document's own .meta quotes the value in a validation error.
	docMeta := readMeta(doc.MetadataPath)
	docMeta["schema_info"].(map[string]any)["validation_errors"] = []any{`conflicting values "` + secretC + `"`}
	quoted, err := json.Marshal(docMeta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(doc.MetadataPath, quoted, 0o600))

	raw := filepath.Join(f.ws.DataDir, "raw", "2026", "11", "01")
	require.NoError(t, os.MkdirAll(raw, 0o700))
	snapshotID := "snap_babab-dodod"
	snapshotFile := filepath.Join(raw, snapshotID+"_snapshot.json")
	snapshotJSON := []byte(`{"errors":["record 0: login failed for ` + secretC + `"],"snapshot_id":"` + snapshotID + `"}`)
	require.NoError(t, os.WriteFile(snapshotFile, snapshotJSON, 0o600))
	collection, hash := "collection", hashOf(snapshotJSON)
	require.NoError(t, db.AddEntry(database.CatalogEntry{ID: snapshotID, StoredPath: snapshotFile, Format: "json", Origin: "observe",
		Schema: "pudl/mu.#ObserveSnapshot", CollectionType: &collection, ContentHash: &hash, ImportTimestamp: time.Now()}))
	require.NoError(t, db.RecordObserveSnapshot(database.ObserveSnapshot{SnapshotID: snapshotID, Source: "command", CreatedAt: time.Now()}))

	otherFile := filepath.Join(raw, "note.json")
	otherMeta := filepath.Join(f.ws.DataDir, "metadata", "note.meta")
	require.NoError(t, os.WriteFile(otherFile, []byte(`{"note":"x"}`), 0o600))
	require.NoError(t, os.WriteFile(otherMeta, []byte(`{"source_info":{"origin":"copied from `+secretC+`"}}`), 0o600))
	otherHash := hashOf([]byte(`{"note":"x"}`))
	require.NoError(t, db.AddEntry(database.CatalogEntry{ID: otherHash, StoredPath: otherFile, MetadataPath: otherMeta, Format: "json",
		Origin: "note", Schema: "pudl/core.#Item", ContentHash: &otherHash, ImportTimestamp: time.Now()}))

	outcome, err := s.Apply(context.Background(), false)
	require.NoError(t, err)
	require.Empty(t, outcome.Errors)
	kinds := map[string]bool{}
	for _, subject := range outcome.Plan.Subjects() {
		kinds[subject.Kind] = true
	}
	assert.Equal(t, map[string]bool{"document": true, "legacy": true, "metadata": true}, kinds)
	assert.Empty(t, f.plaintextFiles(secretC))

	snapshot, err := db.GetEntry(snapshotID)
	require.NoError(t, err, "a snapshot keeps its pre-allocated ID")
	stored, err := os.ReadFile(snapshot.StoredPath)
	require.NoError(t, err)
	assert.Equal(t, hashOf(stored), *snapshot.ContentHash)
	assert.Contains(t, string(stored), "[REDACTED]")
	_, err = db.GetObserveSnapshot(snapshotID)
	require.NoError(t, err)

	other, err := db.GetEntry(otherHash)
	require.NoError(t, err, "a .meta-only rewrite keeps the entry's ID and payload")
	assert.Equal(t, otherFile, other.StoredPath)
	assert.NotEqual(t, otherMeta, other.MetadataPath)

	scrubbed, err := db.GetEntry(mustResolve(t, db, doc.ID))
	require.NoError(t, err)
	meta, err := os.ReadFile(scrubbed.MetadataPath)
	require.NoError(t, err)
	assert.Contains(t, string(meta), "details withheld", "validation messages are withheld as import withholds them")
	assert.Equal(t, `{"name":"solo"}`, *scrubbed.IdentityJSON, "identity is unchanged")

	assert.Equal(t, "ok", doctor.CheckPayloadsAt(context.Background(), f.ws.Root).Status, "payload hashes verify after the scrub")
}
