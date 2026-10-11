package mubridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sensitiveSchemaDir writes a schema module with one sensitive schema.
func sensitiveSchemaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"), []byte("module: \"test.schemas\"\nlanguage: version: \"v0.14.0\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schemas", "custom.cue"), []byte(`package schemas

#Service: {
	_pudl: {
		schema_type: "base"
		resource_type: "provider.service"
		identity_fields: ["name"]
		sensitive_fields: ["env[*].value"]
		facts: svc_env: {each: "env[*]", args: {var: "name", value: {path: "value", default: ""}}}
	}
	name: string
	env?: [...{name: string, value?: string}]
	...
}

`), 0o644))
	return dir
}

func TestIngestObserveManualSchemaRedactsBeforeStorage(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	dir := sensitiveSchemaDir(t)
	inferrer, err := inference.NewSchemaInferrer(dir)
	require.NoError(t, err)
	chain, err := validator.NewChainValidator(dir)
	require.NoError(t, err)

	input := `[{"target":"//models/svc:populate","current":{"records":[{"name":"api","env":[{"name":"TOKEN","value":"s3cr3t"}]}]}}]`
	result, err := IngestObserve(db, ObserveIngest{
		Reader: strings.NewReader(input), DataDir: dataDir,
		Graph: inferrer.GetInheritanceGraph(), Inferrer: inferrer,
		ManualSchema: "schemas.#Service", Chain: chain,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Redacted)

	entry, err := db.GetLatestObserve("models/svc:populate")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "schemas.#Service", entry.Schema)
	stored, err := os.ReadFile(entry.StoredPath)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), "s3cr3t")
	assert.Contains(t, string(stored), "[REDACTED]")
}

func TestIngestObserveProjectsRedactedFactsAndReobservesUnchanged(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	dir := sensitiveSchemaDir(t)
	inferrer, err := inference.NewSchemaInferrer(dir)
	require.NoError(t, err)
	chain, err := validator.NewChainValidator(dir)
	require.NoError(t, err)
	ingest := func(record string) ObserveIngestResult {
		input := `[{"target":"//models/svc:populate","current":{"records":[` + record + `]}}]`
		result, err := IngestObserve(db, ObserveIngest{
			Reader: strings.NewReader(input), DataDir: dataDir,
			Graph: inferrer.GetInheritanceGraph(), Inferrer: inferrer,
			ManualSchema: "schemas.#Service", Chain: chain,
		})
		require.NoError(t, err)
		return result
	}
	values := func() []string {
		facts, err := db.QueryFacts(database.FactFilter{Relation: "svc_env"})
		require.NoError(t, err)
		var out []string
		for _, f := range facts {
			out = append(out, f.Args)
		}
		return out
	}

	a := `{"name":"api","env":[{"name":"TOKEN","value":"s3cr3t"}]}`
	b := `{"name":"api","env":[{"name":"MODE"}]}`
	first := ingest(a)
	assert.Equal(t, map[string]int{"svc_env": 1}, first.Facts)
	require.Len(t, values(), 1)
	assert.Contains(t, values()[0], `"value":"[REDACTED]"`)
	assert.NotContains(t, values()[0], "s3cr3t")

	ingest(b)
	require.Len(t, values(), 1)
	assert.Contains(t, values()[0], `"var":"MODE"`)

	ingest(a) // reverts: deduplicates to the first entry, whose facts return
	require.Len(t, values(), 1)
	assert.Contains(t, values()[0], `"var":"TOKEN"`)
}

func TestStrictObservationRejectsWholeBatchAndPermissiveRedacts(t *testing.T) {
	db, dataDir := setupIngestTestDB(t)
	defer db.Close()
	dir := sensitiveSchemaDir(t)
	inferrer, err := inference.NewSchemaInferrer(dir)
	require.NoError(t, err)
	chain, err := validator.NewChainValidator(dir)
	require.NoError(t, err)
	input := `[{"target":"//svc","current":{"records":[{"name":"valid"},{"name":42,"env":[{"name":"TOKEN","value":"s3cr3t"}]}]}}]`
	in := ObserveIngest{DataDir: dataDir, Inferrer: inferrer, Graph: inferrer.GetInheritanceGraph(), Chain: chain, ManualSchema: "schemas.#Service", Scope: "services", Complete: true}
	in.Reader = strings.NewReader(input)
	_, err = IngestObserve(db, in)
	require.ErrorContains(t, err, "does not satisfy requested schema")
	require.NotContains(t, err.Error(), "s3cr3t")
	rows, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	require.Empty(t, rows.Entries)
	in.AllowSchemaFallback = true
	in.Reader = strings.NewReader(input)
	result, err := IngestObserve(db, in)
	require.NoError(t, err)
	require.Equal(t, 1, result.SchemaMismatches)
	snapshot, err := db.GetObserveSnapshot(result.SnapshotID)
	require.NoError(t, err)
	require.False(t, snapshot.Complete)
	entries, err := db.SnapshotRecordEntries(result.SnapshotID)
	require.NoError(t, err)
	for _, entry := range entries {
		data, err := os.ReadFile(entry.StoredPath)
		require.NoError(t, err)
		require.NotContains(t, string(data), "s3cr3t")
	}
}
