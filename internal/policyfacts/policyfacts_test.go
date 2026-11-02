package policyfacts_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/evidence"
	"github.com/chazu/pudl/internal/importer"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/observe"
	"github.com/chazu/pudl/internal/policy"
	"github.com/chazu/pudl/internal/policyfacts"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/validator"
)

const bucketSchemas = `package aws

#Bucket: {
	_pudl: {schema_type: "base", resource_type: "aws.bucket", identity_fields: ["Name"]}
	Name: string
	...
}

#BucketBestPractice: {
	_pudl: {schema_type: "policy", base_schema: "pudl/aws.#Bucket"}
	PublicAccessBlock: {
		BlockPublicAcls: true @pudl(rule="block-public-acls", severity="high")
	} @pudl(absent="violation")
	Encryption: Algorithm: "aws:kms"
}
`

// fixture is a catalog plus a schema module with a type and its policy.
func fixture(t *testing.T) (*database.CatalogDB, *inference.SchemaInferrer, string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		path := filepath.Join(dir, "schema", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("cue.mod/module.cue", "module: \"pudl.schemas\"\nlanguage: version: \"v0.14.0\"\n")
	write("pudl/aws/aws.cue", bucketSchemas)
	validator.ResetSharedLoaders()
	t.Cleanup(validator.ResetSharedLoaders)
	inferrer, err := inference.NewSchemaInferrer(filepath.Join(dir, "schema"))
	require.NoError(t, err)
	require.Empty(t, inferrer.LoadErrors())
	db, err := database.NewCatalogDB(filepath.Join(dir, "db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, inferrer, filepath.Join(dir, "data")
}

func ingest(t *testing.T, db *database.CatalogDB, inferrer *inference.SchemaInferrer, dataDir string, complete bool, records ...string) observe.ObserveIngestResult {
	t.Helper()
	input := `[{"target":"//aws/buckets","current":{"records":[` + strings.Join(records, ",") + `]}}]`
	result, err := observe.IngestObserve(db, observe.ObserveIngest{
		Reader: strings.NewReader(input), DataDir: dataDir, Scope: "aws/buckets", Complete: complete,
		Graph: inferrer.GetInheritanceGraph(), Inferrer: inferrer,
	})
	require.NoError(t, err)
	return result
}

// statuses maps resource names' current policy_status to their status.
func statuses(t *testing.T, db *database.CatalogDB) map[string]string {
	t.Helper()
	facts, err := db.QueryFacts(database.FactFilter{Relation: database.PolicyStatusRelation})
	require.NoError(t, err)
	out := map[string]string{}
	for _, f := range facts {
		var args map[string]string
		require.NoError(t, json.Unmarshal([]byte(f.Args), &args))
		out[args["resource"]] = args["status"]
		assert.True(t, strings.HasPrefix(f.Source, database.PolicySourcePrefix+"pudl/aws.#BucketBestPractice:"), f.Source)
	}
	return out
}

const (
	good = `{"_schema":"aws.bucket","Name":"good","PublicAccessBlock":{"BlockPublicAcls":true},"Encryption":{"Algorithm":"aws:kms"}}`
	open = `{"_schema":"aws.bucket","Name":"open","PublicAccessBlock":{"BlockPublicAcls":false}}`
)

func TestObserveWritesResultsAndScopeExitClosesThem(t *testing.T) {
	db, inferrer, dataDir := fixture(t)
	first := ingest(t, db, inferrer, dataDir, true, good, open)
	assert.Equal(t, 2, first.PolicyChange.Updated)

	got := statuses(t, db)
	require.Len(t, got, 2)
	var statusList []string
	for _, s := range got {
		statusList = append(statusList, s)
	}
	assert.ElementsMatch(t, []string{"compliant", "noncompliant"}, statusList)

	// Datalog sees both relations and the shipped policy_noncompliant rule.
	rules, err := datalog.LoadRulesFromPaths(filepath.Join("..", "importer", "bootstrap", "pudl", "rules"))
	require.NoError(t, err)
	tuples, err := datalog.Evaluate(db, rules, "policy_noncompliant", nil, datalog.TemporalScope{})
	require.NoError(t, err)
	require.Len(t, tuples, 1)
	assert.Equal(t, "high", tuples[0].Args["severity"])
	tuples, err = datalog.Evaluate(db, rules, "policy_violation", map[string]any{"rule": "block-public-acls"}, datalog.TemporalScope{})
	require.NoError(t, err)
	require.Len(t, tuples, 1)
	assert.Equal(t, "conflict", tuples[0].Args["kind"])
	assert.NotContains(t, tuples[0].Args, "value", "violations never carry values")

	// An ordinary check over the built-in rule needs no producing schema.
	reg := projection.NewRegistry(inferrer, nil)
	assert.Empty(t, projection.CheckDiagnostics(db, reg, rules, "policy_noncompliant", projection.SyncReport{}, nil))

	// A partial observation without "open" proves nothing about it.
	ingest(t, db, inferrer, dataDir, false, good)
	assert.Len(t, statuses(t, db), 2)

	// A complete observation without "open": it left the population.
	last := ingest(t, db, inferrer, dataDir, true, good)
	assert.Equal(t, 1, last.PolicyChange.Closed)
	assert.Len(t, statuses(t, db), 1)
	violations, err := db.QueryFacts(database.FactFilter{Relation: database.PolicyViolationRelation})
	require.NoError(t, err)
	assert.Empty(t, violations)
	history, err := db.FactHistory(database.PolicyViolationRelation)
	require.NoError(t, err)
	assert.NotEmpty(t, history, "closed results stay in history")

	// A check selecting evidence sees results for the selected records only.
	for selector, want := range map[string]int{"scope:aws/buckets": 1, first.SnapshotID: 2} {
		view, err := evidence.Open(context.Background(), db, reg, evidence.Request{Selectors: []string{selector}})
		require.NoError(t, err)
		require.Empty(t, view.Diagnostics)
		assert.Len(t, statuses(t, view.DB), want, selector)
		view.Close()
	}
}

func loadPayload(entry database.CatalogEntry) (any, error) {
	return importer.DecodeStoredFile(entry.StoredPath, entry.Format)
}

func TestRefreshClosesOrphansAndPreviewWritesNothing(t *testing.T) {
	db, inferrer, dataDir := fixture(t)
	ingest(t, db, inferrer, dataDir, false, good, open)
	ev := policyfacts.New(inferrer)
	reasons, err := ev.Stale(db)
	require.NoError(t, err)
	assert.Empty(t, reasons)

	var facts int
	count := func() int {
		require.NoError(t, db.DB().QueryRow(`SELECT COUNT(*) FROM facts`).Scan(&facts))
		return facts
	}
	before := count()
	report, err := ev.Preview(context.Background(), db, loadPayload, ev.Set().Policies())
	require.NoError(t, err)
	assert.Equal(t, 2, report.Resources)
	assert.Equal(t, before, count(), "a preview never writes")

	entry, err := db.GetLatestObserve("aws/buckets")
	require.NoError(t, err)
	require.NoError(t, db.DeleteEntry(entry.ID))
	reasons, err = ev.Stale(db)
	require.NoError(t, err)
	assert.NotEmpty(t, reasons)

	report, err = ev.Refresh(context.Background(), db, loadPayload, policyfacts.Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Orphaned)
	assert.Len(t, statuses(t, db), 1)
}

func TestFactsCarryNoValuesAndUnknownIsNotAViolation(t *testing.T) {
	r := &policy.Result{
		Policy: "p.#P", Type: "p.#T", Digest: "sha256:x", Status: policy.StatusUnknown,
		Unknown: []policy.Violation{{Rule: "enc", Severity: policy.SeverityLow, Path: "Encryption", Kind: policy.KindAbsent}},
	}
	facts := policyfacts.Facts("rid", "eid", r)
	require.Len(t, facts, 1, "an unknown absence is not a violation fact")
	assert.JSONEq(t, `{"resource":"rid","policy":"p.#P","type":"p.#T","status":"unknown","policy_digest":"sha256:x"}`, facts[0].Args)
	assert.JSONEq(t, `{"entry_id":"eid","type":"p.#T"}`, facts[0].Provenance)
}

func TestPolicyRelationsAreReserved(t *testing.T) {
	db, _, _ := fixture(t)
	for _, f := range []database.Fact{
		{Relation: database.PolicyStatusRelation, Args: `{"resource":"r"}`},
		{Relation: database.PolicyViolationRelation, Args: `{"resource":"r"}`, Source: database.PolicySource("p", "r")},
		{Relation: "anything", Args: `{}`, Source: database.PolicySource("p", "r")},
	} {
		_, err := db.AddFact(f)
		assert.Error(t, err, f.Relation)
	}
	assert.True(t, database.IsReservedRelation(database.PolicyStatusRelation))
	assert.True(t, database.IsReservedRelation(database.PolicyViolationRelation))
}
