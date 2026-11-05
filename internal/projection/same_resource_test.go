package projection

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/sameresource"
	"github.com/chazu/pudl/internal/validator"
)

func sameSpec(t *testing.T, raw string) *sameresource.Spec {
	t.Helper()
	spec, err := sameresource.Parse([]byte(raw))
	require.NoError(t, err)
	return spec
}

// clusterSource is two cluster types of different providers that share the
// k8s.cluster kind, and a child type that inherits the GKE declaration.
func clusterSource(t *testing.T) fakeSource {
	return fakeSource{
		"gcp.#GKECluster": {SchemaType: "base", IdentityFields: []string{"selfLink"},
			SameResource: sameSpec(t, `[{"kind": "k8s.cluster", "key": {"host": "endpoint"}}, {"kind": "k8s.cluster", "key": {"host": "privateClusterConfig.privateEndpoint"}}]`)},
		"gcp.#AutopilotCluster": {SchemaType: "base", BaseSchema: "gcp.#GKECluster", IdentityFields: []string{"selfLink"}},
		"k8s.#Cluster": {SchemaType: "base", IdentityFields: []string{"context"},
			SameResource: sameSpec(t, `[{"kind": "k8s.cluster", "key": {"host": {"path": "server", "trim_prefix": "https://"}}}]`)},
	}
}

func keyArgs(t *testing.T, facts []database.Fact) []map[string]string {
	t.Helper()
	var out []map[string]string
	for _, f := range facts {
		require.Equal(t, database.SameResourceKeyRelation, f.Relation)
		var args map[string]string
		require.NoError(t, json.Unmarshal([]byte(f.Args), &args))
		out = append(out, args)
	}
	return out
}

func TestComputeProjectsSameResourceKeysAlongBaseSchema(t *testing.T) {
	reg := NewRegistry(clusterSource(t), nil)
	spec := reg.For("gcp.#AutopilotCluster")
	require.NotNil(t, spec, "an inherited same_resource makes the schema project")
	require.NoError(t, spec.Err)

	res := Compute(spec, "e1", "r1", map[string]any{
		"selfLink": "projects/p/locations/l/clusters/c", "endpoint": "34.1.2.3",
		"privateClusterConfig": map[string]any{"privateEndpoint": "10.0.0.2"},
	})
	assert.Equal(t, []map[string]string{
		{"entry_id": "e1", "resource_id": "r1", "kind": "k8s.cluster", "key": `{"host":"10.0.0.2"}`},
		{"entry_id": "e1", "resource_id": "r1", "kind": "k8s.cluster", "key": `{"host":"34.1.2.3"}`},
	}, keyArgs(t, res.Facts))

	assert.Empty(t, Compute(spec, "e2", "r2", map[string]any{"selfLink": "x"}).Facts, "no key, no fact")

	args := reg.RelationArgs()
	assert.Equal(t, map[string]bool{"entry_id": true, "resource_id": true, "kind": true, "key": true}, args[database.SameResourceKeyRelation])
	assert.ElementsMatch(t, []string{"gcp.#AutopilotCluster", "gcp.#GKECluster", "k8s.#Cluster"}, reg.RelationSchemas()[database.SameResourceKeyRelation])
}

func TestBrokenSameResourceDisablesProjection(t *testing.T) {
	src := clusterSource(t)
	gke := src["gcp.#GKECluster"]
	gke.SameResource, gke.SameResourceError = nil, "same_resource: [0]: unknown field"
	src["gcp.#GKECluster"] = gke
	spec := NewRegistry(src, nil).For("gcp.#AutopilotCluster")
	require.NotNil(t, spec)
	require.Error(t, spec.Err)
	assert.Contains(t, spec.Err.Error(), "schema gcp.#GKECluster: same_resource: [0]: unknown field")
	assert.Contains(t, NewRegistry(src, nil).RelationSchemas()[database.SameResourceKeyRelation], "gcp.#GKECluster",
		"a broken declaration still owns the relation, so old stored facts do not look healthy")
}

func TestSameResourceWithoutIdentityIsAnError(t *testing.T) {
	src := fakeSource{"x.#Host": validator.SchemaMetadata{SchemaType: "base",
		SameResource: sameSpec(t, `[{"kind": "k8s.cluster", "key": {"host": "endpoint"}}]`)}}
	spec := NewRegistry(src, nil).For("x.#Host")
	require.NotNil(t, spec)
	require.Error(t, spec.Err)
	assert.Contains(t, spec.Err.Error(), "declares same_resource keys but no identity_fields")
}

func TestSameResourceChangesTheFingerprint(t *testing.T) {
	src := clusterSource(t)
	before := NewRegistry(src, nil).For("k8s.#Cluster").Fingerprint
	k := src["k8s.#Cluster"]
	k.SameResource = sameSpec(t, `[{"kind": "k8s.cluster", "key": {"host": "server"}}]`)
	src["k8s.#Cluster"] = k
	assert.NotEqual(t, before, NewRegistry(src, nil).For("k8s.#Cluster").Fingerprint)
}

// End to end: records of two schemas with an equal key are linked by the
// built-in same_resource_by_key rule; a record with a different key is not.
func TestSameResourceByKeyLinksProjectedRecords(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	add := func(id, rid, schema, identity, payload string) {
		path := t.TempDir() + "/" + id + ".json"
		require.NoError(t, os.WriteFile(path, []byte(payload), 0o644))
		require.NoError(t, db.AddEntry(database.CatalogEntry{
			ID: id, StoredPath: path, MetadataPath: path + ".meta", ImportTimestamp: time.Now(),
			Format: "json", Origin: "t", Schema: schema, Confidence: 1,
			ResourceID: &rid, ContentHash: &id, IdentityJSON: &identity,
		}))
	}
	add("e-gke", "r-gke", "gcp.#GKECluster", `{"selfLink":"c1"}`, `{"selfLink":"c1","endpoint":"34.1.2.3"}`)
	add("e-ctx", "r-ctx", "k8s.#Cluster", `{"context":"prod"}`, `{"context":"prod","server":"https://34.1.2.3"}`)
	add("e-dev", "r-dev", "k8s.#Cluster", `{"context":"dev"}`, `{"context":"dev","server":"https://127.0.0.1:6443"}`)

	_, err = Sync(context.Background(), db, NewRegistry(clusterSource(t), nil), loadJSON, false)
	require.NoError(t, err)

	rules, err := datalog.LoadRulesFromPaths("../importer/bootstrap/pudl/rules")
	require.NoError(t, err)
	tuples, err := datalog.Evaluate(db, rules, database.SameResourceRelation, nil, datalog.TemporalScope{})
	require.NoError(t, err)

	var links []sameresource.Link
	for _, tp := range tuples {
		links = append(links, sameresource.Link{A: tp.Args["a"].(string), B: tp.Args["b"].(string), Kind: tp.Args["kind"].(string)})
	}
	groups, conflicts := sameresource.Build(links, nil)
	assert.Empty(t, conflicts)
	assert.Equal(t, []sameresource.Group{{ID: "r-ctx", Members: []string{"r-ctx", "r-gke"}, Kinds: []string{"k8s.cluster"}}}, groups)

	kinds, err := datalog.Evaluate(db, rules, sameresource.KindRelation, nil, datalog.TemporalScope{})
	require.NoError(t, err)
	assert.Len(t, kinds, 2, "the built-in k8s.cluster and k8s.object kinds")
}

// The database package reserves the relation names sameresource uses.
func TestSameResourceRelationNamesAgree(t *testing.T) {
	assert.Equal(t, database.SameResourceRelation, sameresource.LinkRelation)
	assert.Equal(t, database.SameResourceKeyRelation, sameresource.KeyRelation)
}
