package wiring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/identity"
	"github.com/chazu/pudl/internal/systemmodel"
)

type testSchemas map[string]cue.Value

func (s testSchemas) GetSchemaValue(name string) (cue.Value, bool) {
	value, ok := s[name]
	return value, ok
}

func compileConsumer(t *testing.T, slot string) *systemmodel.ModelTemplate {
	t.Helper()
	ctx := cuecontext.New()
	source := systemmodel.SchemaCUE() + `
consumer: #SystemModel & {
	name: "consumer"
	inputs: subnet_id: ` + slot + `
	bindings: subnet_id: {
		source: {
			model: "network"
			schema: "resources.#Subnet"
			identity: {name: "private"}
		}
		path: "/details/id"
	}
	populate: #PluginObserve & {plugin: "host", differential: false}
	desired: [{"_schema": "example.application", subnet_id: inputs.subnet_id}]
}`
	root := ctx.CompileString(source, cue.Filename("consumer.cue"))
	require.NoError(t, root.Err())
	template, err := systemmodel.NewTemplate(root.LookupPath(cue.ParsePath("consumer")), systemmodel.TemplateOrigin{})
	require.NoError(t, err)
	return template
}

func compileSourceSchema(t *testing.T, annotation string) cue.Value {
	t.Helper()
	ctx := cuecontext.New()
	root := ctx.CompileString(`
#Subnet: {
	name: string
	details: id: string `+annotation+`
}`, cue.Filename("resources.cue"))
	require.NoError(t, root.Err())
	return root.LookupPath(cue.ParsePath("#Subnet"))
}

type resolverFixture struct {
	db      *database.CatalogDB
	rawDir  string
	created time.Time
}

func newResolverFixture(t *testing.T) *resolverFixture {
	t.Helper()
	root := t.TempDir()
	db, err := database.NewCatalogDB(filepath.Join(root, "catalog"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	rawDir := filepath.Join(root, "raw")
	require.NoError(t, os.MkdirAll(rawDir, 0o755))
	return &resolverFixture{db: db, rawDir: rawDir, created: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)}
}

func (f *resolverFixture) addSnapshot(t *testing.T, snapshotID, runID, status string, records ...map[string]any) {
	t.Helper()
	require.NoError(t, f.db.StartRun(runID, "network", "observe-only"))
	snapshot := database.ObserveSnapshot{
		SnapshotID: snapshotID, RunID: runID, Model: "network", Workspace: "repo",
		Origin: "pudl-run", Source: database.SnapshotSourceMuObserve,
		RecordCount: len(records), CreatedAt: f.created,
	}
	require.NoError(t, f.db.RecordObserveSnapshot(snapshot))
	entryType := "observe"
	collection := "collection"
	require.NoError(t, f.db.AddEntry(database.CatalogEntry{
		ID: snapshotID, ImportTimestamp: f.created, Format: "json", Origin: "pudl-run",
		Schema: "pudl/mu.#ObserveSnapshot", EntryType: &entryType, CollectionType: &collection,
	}))
	item := "item"
	for i, record := range records {
		data, err := json.Marshal(record)
		require.NoError(t, err)
		path := filepath.Join(f.rawDir, snapshotID+string(rune('a'+i))+".json")
		require.NoError(t, os.WriteFile(path, data, 0o644))
		identityJSON, err := identity.CanonicalIdentityJSON(map[string]any{"name": record["name"]})
		require.NoError(t, err)
		entryID := snapshotID + "_record_" + string(rune('a'+i))
		require.NoError(t, f.db.AddEntry(database.CatalogEntry{
			ID: entryID, StoredPath: path, ImportTimestamp: f.created, Format: "json",
			Origin: "pudl-run", Schema: "resources.#Subnet", EntryType: &entryType,
			CollectionType: &item, IdentityJSON: &identityJSON,
		}))
		require.NoError(t, f.db.AddCollectionMembership(snapshotID, entryID, i))
	}
	require.NoError(t, f.db.FinishRun(runID, database.RunConclusion{CompletionStatus: status}))
}

func TestResolverElaboratesExactAuthorizedScalar(t *testing.T) {
	fixture := newResolverFixture(t)
	fixture.addSnapshot(t, "snap_ok", "run_network", database.RunStatusSucceeded,
		map[string]any{"name": "private", "details": map[string]any{"id": "subnet-123"}})
	maxAge := time.Hour
	resolver := Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": compileSourceSchema(t, `@pudl(binding=plain)`),
	}}

	result, err := resolver.Elaborate(compileConsumer(t, `string @pudl(binding=plain)`), ResolveRequest{
		Workspace: "repo", MaxObservationAge: &maxAge,
		EvaluationTime: fixture.created.Add(10 * time.Minute),
	})
	require.NoError(t, err)
	require.Len(t, result.Model.Desired, 1)
	assert.Equal(t, "subnet-123", result.Model.Desired[0]["subnet_id"])
	require.Len(t, result.Evidence, 1)
	assert.Equal(t, "reused", result.Evidence[0].Selection)
	assert.Equal(t, "string", result.Evidence[0].ValueType)
	assert.Equal(t, "resolved", result.Evidence[0].ResolutionCode)
	assert.Equal(t, json.RawMessage(`"subnet-123"`), result.Inputs["subnet_id"])

	current, err := resolver.Elaborate(compileConsumer(t, `string @pudl(binding=plain)`), ResolveRequest{
		Workspace: "repo", EvaluationTime: fixture.created.Add(10 * time.Minute),
		CurrentProducerRuns: map[string]ProducerRun{"network": {Model: "network", RunID: "run_network"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "current-run", current.Evidence[0].Selection)

	aliasTemplate := compileConsumer(t, `string @pudl(binding=plain)`)
	aliasBinding := aliasTemplate.Bindings["subnet_id"]
	aliasBinding.Source.Model = "network-alias"
	aliasTemplate.Bindings["subnet_id"] = aliasBinding
	aliased, err := resolver.Elaborate(aliasTemplate, ResolveRequest{
		Workspace: "repo", EvaluationTime: fixture.created.Add(10 * time.Minute),
		CurrentProducerRuns: map[string]ProducerRun{
			"network-alias": {Model: "network", RunID: "run_network"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "network", aliased.Evidence[0].ProducerModel)
	assert.Equal(t, "current-run", aliased.Evidence[0].Selection)

	require.NoError(t, fixture.db.PrepareRunMutation("run_network"))
	fixture.created = fixture.created.Add(time.Hour)
	fixture.addSnapshot(t, "snap_newer", "run_network_new", database.RunStatusSucceeded,
		map[string]any{"name": "private", "details": map[string]any{"id": "subnet-newer"}})
	pinned, err := resolver.Elaborate(aliasTemplate, ResolveRequest{
		Workspace: "repo", EvaluationTime: fixture.created.Add(10 * time.Minute),
		PinnedProducerSnapshots: map[string]PinnedProducerSnapshot{
			"network-alias": {Model: "network", RunID: "run_network", SnapshotID: "snap_ok"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "snap_ok", pinned.Evidence[0].SnapshotID)
	assert.Equal(t, "subnet-123", pinned.Model.Desired[0]["subnet_id"])
	assert.Equal(t, "current-run", pinned.Evidence[0].Selection)
}

func TestResolverElaboratesAuthorizedOptionalSchemaField(t *testing.T) {
	fixture := newResolverFixture(t)
	fixture.addSnapshot(t, "snap_optional", "run_network", database.RunStatusSucceeded,
		map[string]any{"name": "private", "details": map[string]any{"id": "subnet-optional"}})
	ctx := cuecontext.New()
	schema := ctx.CompileString(`
#Subnet: {
	name: string
	details?: id?: string @pudl(binding=plain)
}`, cue.Filename("resources.cue")).LookupPath(cue.ParsePath("#Subnet"))
	require.NoError(t, schema.Err())

	result, err := (Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": schema,
	}}).Elaborate(compileConsumer(t, `string @pudl(binding=plain)`), ResolveRequest{
		Workspace: "repo", EvaluationTime: fixture.created,
	})
	require.NoError(t, err)
	assert.Equal(t, "subnet-optional", result.Model.Desired[0]["subnet_id"])
}

func TestResolverNeverFallsBackBehindNewestSuccessfulSnapshot(t *testing.T) {
	fixture := newResolverFixture(t)
	fixture.addSnapshot(t, "snap_old", "run_old", database.RunStatusSucceeded,
		map[string]any{"name": "private", "details": map[string]any{"id": "subnet-old"}})
	fixture.created = fixture.created.Add(time.Hour)
	fixture.addSnapshot(t, "snap_new", "run_new", database.RunStatusSucceeded)
	resolver := Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": compileSourceSchema(t, `@pudl(binding=plain)`),
	}}

	_, err := resolver.Elaborate(compileConsumer(t, `string @pudl(binding=plain)`), ResolveRequest{
		Workspace: "repo", EvaluationTime: fixture.created,
	})
	var resolutionErr *ResolutionError
	require.ErrorAs(t, err, &resolutionErr)
	assert.Equal(t, "source-absent", resolutionErr.Code)
	assert.Contains(t, resolutionErr.Error(), "snap_new")
}

func TestResolverFailsClosedForDeniedOrAmbiguousSource(t *testing.T) {
	fixture := newResolverFixture(t)
	fixture.addSnapshot(t, "snap_two", "run_network", database.RunStatusSucceeded,
		map[string]any{"name": "private", "details": map[string]any{"id": "one"}},
		map[string]any{"name": "private", "details": map[string]any{"id": "two"}})

	_, err := (Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": compileSourceSchema(t, ""),
	}}).Elaborate(compileConsumer(t, `string @pudl(binding=plain)`), ResolveRequest{Workspace: "repo"})
	var resolutionErr *ResolutionError
	require.ErrorAs(t, err, &resolutionErr)
	assert.Equal(t, "projection-denied", resolutionErr.Code)

	_, err = (Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": compileSourceSchema(t, `@pudl(binding=plain)`),
	}}).Elaborate(compileConsumer(t, `string @pudl(binding=plain)`), ResolveRequest{Workspace: "repo"})
	require.ErrorAs(t, err, &resolutionErr)
	assert.Equal(t, "source-ambiguous", resolutionErr.Code)
}

func TestResolverEnforcesAgeAndFinalCUEType(t *testing.T) {
	fixture := newResolverFixture(t)
	fixture.addSnapshot(t, "snap_ok", "run_network", database.RunStatusSucceeded,
		map[string]any{"name": "private", "details": map[string]any{"id": "subnet-123"}})
	resolver := Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": compileSourceSchema(t, `@pudl(binding=plain)`),
	}}
	maxAge := time.Minute
	_, err := resolver.Elaborate(compileConsumer(t, `string @pudl(binding=plain)`), ResolveRequest{
		Workspace: "repo", MaxObservationAge: &maxAge, EvaluationTime: fixture.created.Add(time.Hour),
	})
	var resolutionErr *ResolutionError
	require.ErrorAs(t, err, &resolutionErr)
	assert.Equal(t, "source-too-old", resolutionErr.Code)

	_, err = resolver.Elaborate(compileConsumer(t, `int @pudl(binding=plain)`), ResolveRequest{
		Workspace: "repo", EvaluationTime: fixture.created,
	})
	require.ErrorAs(t, err, &resolutionErr)
	assert.Equal(t, "type-mismatch", resolutionErr.Code)
}

func TestResolverRejectsMissingAndNonLeafProjectionPaths(t *testing.T) {
	fixture := newResolverFixture(t)
	fixture.addSnapshot(t, "snap_ok", "run_network", database.RunStatusSucceeded,
		map[string]any{"name": "private", "details": map[string]any{"id": "subnet-123"}})

	missing := compileConsumer(t, `string @pudl(binding=plain)`)
	binding := missing.Bindings["subnet_id"]
	binding.Path = "/details/missing"
	missing.Bindings["subnet_id"] = binding
	_, err := (Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": compileSourceSchema(t, `@pudl(binding=plain)`),
	}}).Elaborate(missing, ResolveRequest{Workspace: "repo"})
	var resolutionErr *ResolutionError
	require.ErrorAs(t, err, &resolutionErr)
	assert.Equal(t, "projection-invalid", resolutionErr.Code)
	assert.Contains(t, resolutionErr.Error(), "does not exist")

	nonLeaf := compileConsumer(t, `string @pudl(binding=plain)`)
	binding = nonLeaf.Bindings["subnet_id"]
	binding.Path = "/details"
	nonLeaf.Bindings["subnet_id"] = binding
	ctx := cuecontext.New()
	nonLeafSchema := ctx.CompileString(`
#Subnet: {
	name: string
	details: {id: string} @pudl(binding=plain)
}`, cue.Filename("resources.cue")).LookupPath(cue.ParsePath("#Subnet"))
	require.NoError(t, nonLeafSchema.Err())
	_, err = (Resolver{Catalog: fixture.db, Schemas: testSchemas{
		"resources.#Subnet": nonLeafSchema,
	}}).Elaborate(nonLeaf, ResolveRequest{Workspace: "repo"})
	require.ErrorAs(t, err, &resolutionErr)
	assert.Equal(t, "projection-invalid", resolutionErr.Code)
	assert.Contains(t, resolutionErr.Error(), "scalar leaf")
}
