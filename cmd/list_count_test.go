package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
)

// seedCountCatalog writes catalog entries with the given schema, origin and
// JSON payload, each backed by a stored file.
func seedCountCatalog(t *testing.T, root string, entries []countSeed) {
	t.Helper()
	db, err := database.NewCatalogDB(root)
	require.NoError(t, err)
	defer db.Close()
	raw := filepath.Join(root, "data", "raw", "count")
	require.NoError(t, os.MkdirAll(raw, 0o755))
	now := time.Now().UTC()
	for i, e := range entries {
		data, err := json.Marshal(e.payload)
		require.NoError(t, err)
		path := filepath.Join(raw, e.id+".json")
		require.NoError(t, os.WriteFile(path, data, 0o644))
		require.NoError(t, db.AddEntry(database.CatalogEntry{
			ID: e.id, StoredPath: path, ImportTimestamp: now.Add(time.Duration(i) * time.Second),
			Format: "json", Origin: e.origin, Schema: e.schema, Confidence: 1,
			RecordCount: 1, SizeBytes: int64(len(data)),
		}))
	}
}

type countSeed struct {
	id, schema, origin string
	payload            map[string]any
}

var countSeeds = []countSeed{
	{"countbucket1", "pudl/aws.#Bucket", "aws-s3", map[string]any{"name": "b1", "region": "eu"}},
	{"countbucket2", "pudl/aws.#Bucket", "aws-s3", map[string]any{"name": "b2", "region": "us"}},
	{"countbucket3", "pudl/aws.#Bucket", "aws-s3", map[string]any{"name": "b3", "region": "eu"}},
	{"countrepo1", "pudl/git.#GitRepository", "git", map[string]any{"name": "r1", "region": "eu"}},
	{"countsnap1", "pudl/core.#ObserveSnapshot", "observe", map[string]any{"model": "m"}},
	{"countsnap2", "pudl/core.#ObserveSnapshot", "observe", map[string]any{"model": "m"}},
}

func TestListCountBySchemaText(t *testing.T) {
	seedCountCatalog(t, cliWorkspace(t), countSeeds)

	r := runCLI(t, "list", "--count-by", "schema")
	require.NoError(t, r.Err, r.Stderr)
	require.Equal(t, "6 entries in 3 schemas\n\n"+
		"COUNT  SCHEMA\n"+
		"    3  pudl/aws.#Bucket\n"+
		"    1  pudl/git.#GitRepository\n"+
		"\n"+
		"pudl bookkeeping:\n"+
		"    2  pudl/core.#ObserveSnapshot\n", r.Stdout)
}

func TestListCountByJSONRespectsFilters(t *testing.T) {
	seedCountCatalog(t, cliWorkspace(t), countSeeds)
	count := func(args ...string) listCountOutput {
		t.Helper()
		r := runCLI(t, append([]string{"list", "--json"}, args...)...)
		require.NoError(t, r.Err, r.Stderr)
		var out listCountOutput
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
		return out
	}

	all := count("--count-by", "schema")
	require.Equal(t, "schema", all.CountBy)
	require.Equal(t, 6, all.TotalEntries)
	require.Equal(t, 6, all.TotalMatched)
	require.Equal(t, []map[string]any{
		{"schema": "pudl/aws.#Bucket", "count": float64(3)},
		{"schema": "pudl/core.#ObserveSnapshot", "count": float64(2), "bookkeeping": true},
		{"schema": "pudl/git.#GitRepository", "count": float64(1)},
	}, all.Groups)

	where := count("--count-by", "schema", "--where", "region=eu")
	require.Equal(t, []map[string]any{
		{"schema": "pudl/aws.#Bucket", "count": float64(2)},
		{"schema": "pudl/git.#GitRepository", "count": float64(1)},
	}, where.Groups)

	origin := count("--count-by", "origin", "--schema", "aws")
	require.Equal(t, []map[string]any{{"origin": "aws-s3", "count": float64(3)}}, origin.Groups)

	limited := count("--count-by", "schema", "--origin", "aws-s3", "--limit", "2")
	require.Equal(t, 2, limited.TotalEntries)
	require.Equal(t, 3, limited.TotalMatched)

	none := count("--count-by", "format", "--schema", "nothing")
	require.Equal(t, []map[string]any{}, none.Groups)
	require.Zero(t, none.TotalMatched)
}

func TestListCountByRejectsUnknownFieldAndPaging(t *testing.T) {
	seedCountCatalog(t, cliWorkspace(t), countSeeds)
	r := runCLI(t, "list", "--count-by", "size")
	require.ErrorContains(t, r.Err, `Cannot count by "size"`)
	r = runCLI(t, "list", "--count-by", "schema", "--page", "2")
	require.Error(t, r.Err)
}
