package projection

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSource is a schema set held in memory.
type fakeSource map[string]validator.SchemaMetadata

func (f fakeSource) GetAvailableSchemas() []string {
	var out []string
	for k := range f {
		out = append(out, k)
	}
	return out
}
func (f fakeSource) GetSchemaMetadata(s string) (validator.SchemaMetadata, bool) {
	m, ok := f[s]
	return m, ok
}
func (f fakeSource) GetInheritanceGraph() *inference.InheritanceGraph {
	return inference.BuildInheritanceGraph(f)
}

func hostSource(facts string) fakeSource {
	return fakeSource{"x.#Host": {SchemaType: "base", IdentityFields: []string{"name"}, FactsSpec: json.RawMessage(facts)}}
}

func addHostEntry(t *testing.T, db *database.CatalogDB, id, rid, payload string) {
	t.Helper()
	path := t.TempDir() + "/" + id + ".json"
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o644))
	identity := `{"name":"h"}`
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: id, StoredPath: path, MetadataPath: path + ".meta", ImportTimestamp: time.Now(),
		Format: "json", Origin: "t", Schema: "x.#Host", Confidence: 1,
		ResourceID: &rid, ContentHash: &id, IdentityJSON: &identity,
	}))
}

func loadJSON(entry database.CatalogEntry) (any, error) {
	b, err := os.ReadFile(entry.StoredPath)
	if err != nil {
		return nil, err
	}
	var v any
	return v, json.Unmarshal(b, &v)
}

func hostFacts(t *testing.T, db *database.CatalogDB) []string {
	facts, err := db.QueryFacts(database.FactFilter{Relation: "host"})
	require.NoError(t, err)
	var out []string
	for _, f := range facts {
		out = append(out, f.Args)
	}
	return out
}

func TestSyncBootstrapsReprojectsAndRetiresOrphans(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	addHostEntry(t, db, "e1", "rid1", `{"name":"h","os":"linux"}`)

	reg := NewRegistry(hostSource(`{"host": {"args": {"name": "name"}}}`), nil)
	reasons, err := Stale(db, reg)
	require.NoError(t, err)
	assert.NotEmpty(t, reasons, "an unprojected resource is stale")

	report, err := Sync(ctx, db, reg, loadJSON, false)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Bootstrapped)
	require.Len(t, hostFacts(t, db), 1)

	again, err := Sync(ctx, db, reg, loadJSON, false)
	require.NoError(t, err)
	assert.False(t, again.Changed(), "a second sync is a no-op")

	// The spec changes: facts are recomputed.
	reg = NewRegistry(hostSource(`{"host": {"args": {"name": "name", "os": "os"}}}`), nil)
	report, err = Sync(ctx, db, reg, loadJSON, false)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Reprojected)
	require.Len(t, hostFacts(t, db), 1)
	assert.Contains(t, hostFacts(t, db)[0], `"os":"linux"`)

	// A broken spec leaves facts alone.
	broken := NewRegistry(hostSource(`{"host": {"args": {"name": "name", "a": "x[*]", "b": "y[*]"}}}`), nil)
	report, err = Sync(ctx, db, broken, loadJSON, false)
	require.NoError(t, err)
	assert.NotEmpty(t, report.Broken)
	require.Len(t, hostFacts(t, db), 1)

	// The entry is deleted: its facts are retired.
	err = db.DeleteEntry("e1")
	require.NoError(t, err)
	report, err = Sync(ctx, db, reg, loadJSON, false)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Orphaned)
	assert.Empty(t, hostFacts(t, db))
}

func TestParseRelationsRejectsMistakes(t *testing.T) {
	for name, spec := range map[string]string{
		"two wildcards":    `{"r": {"args": {"a": "x[*]", "b": "y[*]"}}}`,
		"reserved":         `{"catalog_entry": {"args": {"a": "x"}}}`,
		"implicit arg":     `{"r": {"args": {"entry_id": "x"}}}`,
		"bad name":         `{"r-1": {"args": {"a": "x"}}}`,
		"each no wildcard": `{"r": {"each": "x", "args": {"a": "y"}}}`,
		"unknown key":      `{"r": {"args": {"a": {"path": "x", "dflt": 1}}}}`,
		"no args":          `{"r": {"args": {}}}`,
	} {
		_, err := ParseRelations(json.RawMessage(spec))
		assert.Error(t, err, name)
	}
}

func TestComputeExistsDefaultAndNumbers(t *testing.T) {
	rels, err := ParseRelations(json.RawMessage(`{"gke": {"args": {
		"name": "name",
		"private": {"exists": "privateClusterConfig"},
		"rotation": {"path": "rotationPeriod", "default": ""},
		"big": "big"
	}}}`))
	require.NoError(t, err)
	spec := &SchemaSpec{Schema: "s", Relations: rels}
	var data any
	dec := json.NewDecoder(strings.NewReader(`{"name":"c","big":123456789012345678901234567890}`))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&data))
	res := Compute(spec, "e", "r", data)
	require.Len(t, res.Facts, 1)
	assert.JSONEq(t, `{"name":"c","private":false,"rotation":"","entry_id":"e","resource_id":"r"}`, res.Facts[0].Args)
	assert.Len(t, res.Omitted, 1, "an out-of-range number is omitted and reported")
}
