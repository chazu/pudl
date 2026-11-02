package scrub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/test/testutil"
)

const loginSchema = `package vault

#Login: {
	_pudl: {
		schema_type:   "base"
		resource_type: "test.login"
		identity_fields: ["name"]
		%s
		facts: vault_login: args: {name: "name", password: "password"}
	}
	name: string
	...
}
`

// Values long enough for the value search, and one too short for it.
const (
	secretA = "hunter2-correct-horse"
	secretB = "battery-staple-9000"
	secretC = "solo-document-secret"
)

type fixture struct {
	t         *testing.T
	ws        *testutil.PUDLWorkspace
	setup     *testutil.TempDirSetup
	schemaDir string
}

// newFixture stages a workspace whose vault.#Login schema does not yet
// declare password sensitive.
func newFixture(t *testing.T) *fixture {
	setup := testutil.NewTempDirSetup(t)
	ws := setup.CreatePUDLWorkspace()
	setup.AddBootstrapSchemas(ws)
	f := &fixture{t: t, ws: ws, setup: setup, schemaDir: filepath.Join(ws.SchemaDir, "pudl", "vault")}
	f.writeSchema("")
	return f
}

func (f *fixture) writeSchema(sensitive string) {
	require.NoError(f.t, os.MkdirAll(f.schemaDir, 0o755))
	require.NoError(f.t, os.WriteFile(filepath.Join(f.schemaDir, "vault.cue"), []byte(fmt.Sprintf(loginSchema, sensitive)), 0o644))
	inference.ResetShared()
	validator.ResetSharedLoaders()
}

// declare makes password sensitive, as a schema edit after import would.
func (f *fixture) declare() { f.writeSchema(`sensitive_fields: ["password"]`) }

func (f *fixture) importFile(name, content string) *importer.ImportResult {
	imp, err := importer.NewEnhancedImporter(f.ws.DataDir, f.ws.SchemaDir, f.ws.Root)
	require.NoError(f.t, err)
	defer imp.Close()
	result, err := imp.ImportFileWithFriendlyIDs(importer.ImportOptions{SourcePath: f.setup.WriteFile(name, content), ManualSchema: "pudl/vault.#Login"})
	require.NoError(f.t, err)
	return result
}

func (f *fixture) scrubber() (*Scrubber, *database.CatalogDB) {
	db, err := database.NewCatalogDB(f.ws.Root)
	require.NoError(f.t, err)
	f.t.Cleanup(func() { db.Close() })
	inferrer, err := inference.NewSchemaInferrer(f.ws.SchemaDir)
	require.NoError(f.t, err)
	return &Scrubber{DB: db, Inferrer: inferrer, Projections: projection.NewRegistry(inferrer, nil)}, db
}

// state dumps the catalog tables and data files the scrub may change.
func (f *fixture) state(db *database.CatalogDB) string {
	var out []string
	for _, query := range []string{
		`SELECT id, stored_path, metadata_path, content_hash, resource_id, item_id, origin, size_bytes, updated_at FROM catalog_entries ORDER BY id`,
		`SELECT collection_id, item_id, item_index FROM collection_memberships ORDER BY 1, 2`,
		`SELECT resource_id, entry_id, fingerprint FROM projection_state ORDER BY 1`,
		`SELECT id, args, provenance, tx_end FROM facts ORDER BY id`,
		`SELECT run_id, report_json FROM run_reports ORDER BY 1`,
		`SELECT scrub_id, compacted_at FROM sensitive_scrubs ORDER BY 1`,
	} {
		rows, err := db.DB().Query(query)
		require.NoError(f.t, err)
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			require.NoError(f.t, rows.Scan(ptrs...))
			out = append(out, fmt.Sprint(values...))
		}
		rows.Close()
	}
	for path, sum := range f.files() {
		out = append(out, path+" "+sum)
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// files maps each raw and metadata file to its content hash.
func (f *fixture) files() map[string]string {
	out := map[string]string{}
	for _, dir := range []string{"raw", "metadata"} {
		_ = filepath.WalkDir(filepath.Join(f.ws.DataDir, dir), func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				b, _ := os.ReadFile(path)
				sum := sha256.Sum256(b)
				out[path] = hex.EncodeToString(sum[:])
			}
			return nil
		})
	}
	return out
}

// plaintextFiles lists the data files that still contain a secret.
func (f *fixture) plaintextFiles(secrets ...string) []string {
	var out []string
	for path := range f.files() {
		b, _ := os.ReadFile(path)
		for _, s := range secrets {
			if strings.Contains(string(b), s) {
				out = append(out, path)
			}
		}
	}
	return out
}

func entryIDs(t *testing.T, db *database.CatalogDB) map[string]database.CatalogEntry {
	result, err := db.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	out := map[string]database.CatalogEntry{}
	for _, e := range result.Entries {
		out[e.ID] = e
	}
	return out
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestScrubRedactsRederivesIDsAndKeepsHistory(t *testing.T) {
	f := newFixture(t)
	collection := f.importFile("logins.json", `[{"name":"svc","password":"`+secretA+`"},{"name":"db","password":"`+secretB+`"}]`)
	document := f.importFile("solo.json", `{"name":"solo","password":"`+secretC+`"}`)
	f.declare()
	s, db := f.scrubber()
	ctx := context.Background()

	items, err := db.GetCollectionItems(collection.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	oldIDs := []string{collection.ID, document.ID, items[0].ID, items[1].ID}
	report := fmt.Sprintf(`{"evidence_ids":[%q],"observed":"token is %s"}`, items[0].ID, secretA)
	require.NoError(t, db.SaveRunReport("run-1", "m", []byte(report)))
	history, err := db.DB().Query(`SELECT id, args, provenance, valid_start, tx_start FROM facts`)
	require.NoError(t, err)
	var before []string
	for history.Next() {
		var id, args, prov, vs, ts string
		require.NoError(t, history.Scan(&id, &args, &prov, &vs, &ts))
		before = append(before, id+args+prov+vs+ts)
	}
	history.Close()
	require.NotEmpty(t, before)

	// Diagnosis: every artifact is named, no value is.
	plan, err := s.Plan(ctx)
	require.NoError(t, err)
	kinds := map[string]int{}
	for _, subject := range plan.Subjects() {
		kinds[subject.Kind]++
	}
	assert.Equal(t, map[string]int{"item": 2, "document": 1, "collection": 1, "run_report": 1}, kinds)
	printed := fmt.Sprintf("%+v %+v %v", plan.Subjects(), plan.Unsupported, plan.Errors)
	for _, secret := range []string{secretA, secretB, secretC} {
		assert.NotContains(t, printed, secret)
	}

	// A dry run writes nothing.
	snapshot := f.state(db)
	_, err = s.Apply(ctx, true)
	require.NoError(t, err)
	require.Equal(t, snapshot, f.state(db))

	outcome, err := s.Apply(ctx, false)
	require.NoError(t, err)
	require.Empty(t, outcome.Errors)
	assert.True(t, outcome.Compacted)
	assert.Empty(t, f.plaintextFiles(secretA, secretB, secretC), "payloads, items, collection and .meta are redacted")

	// IDs are re-derived from the redacted content, and every reference follows.
	entries := entryIDs(t, db)
	for _, old := range oldIDs {
		assert.NotContains(t, entries, old)
		current, ok, err := db.ResolveEntryRedirect(old)
		require.NoError(t, err)
		require.True(t, ok, "old ID %s resolves", old)
		require.Contains(t, entries, current)
		entry := entries[current]
		stored, err := os.ReadFile(entry.StoredPath)
		require.NoError(t, err)
		if entry.CollectionType != nil && *entry.CollectionType == "item" {
			canonical, err := importer.EncodeRecord("", 0, mustDecode(t, stored))
			require.NoError(t, err)
			assert.Equal(t, hashOf(canonical), current, "an item is identified by its canonical record")
		} else {
			assert.Equal(t, hashOf(stored), current, "a file is identified by its bytes")
		}
		assert.Equal(t, current, *entry.ContentHash)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM collection_memberships WHERE collection_id IN (?,?,?,?) OR item_id IN (?,?,?,?)`,
		`SELECT COUNT(*) FROM projection_state WHERE entry_id IN (?,?,?,?)`,
		`SELECT COUNT(*) FROM catalog_entries WHERE item_id IN (?,?,?,?)`,
	} {
		args := []any{}
		for range strings.Count(query, "?") / 4 {
			for _, id := range oldIDs {
				args = append(args, id)
			}
		}
		var n int
		require.NoError(t, db.DB().QueryRow(query, args...).Scan(&n))
		assert.Zero(t, n, query)
	}
	newItems, err := db.GetCollectionItems(entries[mustResolve(t, db, collection.ID)].ID)
	require.NoError(t, err)
	assert.Len(t, newItems, 2, "memberships follow both re-keyed sides")

	// Current facts are re-projected under the new IDs; history is untouched.
	var plaintextCurrent, newCurrent int
	require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM current_facts WHERE args LIKE ? OR args LIKE ?`, "%"+secretA+"%", "%"+secretC+"%").Scan(&plaintextCurrent))
	require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM current_facts WHERE relation = 'vault_login' AND json_extract(args, '$.entry_id') = ? AND json_extract(args, '$.password') = '[REDACTED]'`,
		mustResolve(t, db, document.ID)).Scan(&newCurrent))
	assert.Zero(t, plaintextCurrent)
	assert.Equal(t, 1, newCurrent)
	for _, row := range before {
		var n int
		require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM facts WHERE id||args||provenance||valid_start||tx_start = ?`, row).Scan(&n))
		assert.Equal(t, 1, n, "a historical fact version is unchanged")
	}

	// The report keeps citing the old ID, which resolves, without the value.
	stored, err := db.GetRunReport("run-1")
	require.NoError(t, err)
	assert.NotContains(t, string(stored.Report), secretA)
	assert.Contains(t, string(stored.Report), items[0].ID)

	// A second run is clean and writes nothing.
	snapshot = f.state(db)
	again, err := s.Plan(ctx)
	require.NoError(t, err)
	assert.Zero(t, again.Pending())
	_, err = s.Apply(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, snapshot, f.state(db))

	// Re-importing the original sources deduplicates against the scrubbed entries.
	reimported := f.importFile("solo-again.json", `{"name":"solo","password":"`+secretC+`"}`)
	assert.True(t, reimported.Skipped)
	assert.Equal(t, mustResolve(t, db, document.ID), reimported.ID)
	recollected := f.importFile("logins-again.json", `[{"name":"svc","password":"`+secretA+`"},{"name":"db","password":"`+secretB+`"}]`)
	assert.True(t, recollected.Skipped)
	assert.Equal(t, mustResolve(t, db, collection.ID), recollected.ID)
}

func TestScrubMergesIntoAnEntryWithTheRederivedID(t *testing.T) {
	f := newFixture(t)
	old := f.importFile("a.json", `{"name":"svc","password":"`+secretA+`"}`)
	// Two entries that differ only in the secret collide with each other.
	twinA := f.importFile("c.json", `{"name":"twin","password":"`+secretA+`"}`)
	twinB := f.importFile("d.json", `{"name":"twin","password":"`+secretB+`"}`)
	f.declare()
	// Imported after the declaration: stored redacted, so it already has the
	// ID the scrub derives for the first one.
	survivor := f.importFile("b.json", `{"name":"svc","password":"`+secretB+`"}`)
	s, db := f.scrubber()

	outcome, err := s.Apply(context.Background(), false)
	require.NoError(t, err)
	require.Empty(t, outcome.Errors)
	merged := map[string]Subject{}
	for _, subject := range outcome.Plan.Subjects() {
		merged[subject.ID] = subject
	}
	require.Len(t, merged, 3)
	assert.True(t, merged[old.ID].Merged)
	assert.Equal(t, survivor.ID, merged[old.ID].NewID)
	assert.NotEqual(t, merged[twinA.ID].Merged, merged[twinB.ID].Merged, "the first twin is re-keyed, the second merges into it")
	assert.Equal(t, mustResolve(t, db, twinA.ID), mustResolve(t, db, twinB.ID))

	entries := entryIDs(t, db)
	assert.Len(t, entries, 2)
	assert.NotContains(t, entries, old.ID)
	assert.Contains(t, entries, survivor.ID)
	assert.Equal(t, survivor.ID, mustResolve(t, db, old.ID))
	assert.NoFileExists(t, old.StoredPath)
	assert.NoFileExists(t, old.MetadataPath)
	assert.Empty(t, f.plaintextFiles(secretA))
}

func mustResolve(t *testing.T, db *database.CatalogDB, id string) string {
	t.Helper()
	current, ok, err := db.ResolveEntryRedirect(id)
	require.NoError(t, err)
	require.True(t, ok)
	return current
}

func mustDecode(t *testing.T, b []byte) any {
	t.Helper()
	v, err := idgen.DecodeJSONExact(b)
	require.NoError(t, err)
	return v
}
