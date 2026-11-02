package projection

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/ownership"
	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ownerSpec(t *testing.T, raw string) *ownership.Spec {
	t.Helper()
	spec, err := ownership.Parse([]byte(raw))
	require.NoError(t, err)
	return spec
}

// ownedSource is a parent type declaring ownership and a child type that
// inherits it along base_schema.
func ownedSource(t *testing.T) fakeSource {
	return fakeSource{
		"x.#Host": {SchemaType: "base", IdentityFields: []string{"name"}, Owner: ownerSpec(t, `{"paths": ["labels.team"], "map": {"path": "project", "owners": {"proj-a": "payments"}}}`)},
		"x.#Web":  {SchemaType: "base", BaseSchema: "x.#Host", IdentityFields: []string{"name"}},
	}
}

func ownerArgs(t *testing.T, facts []database.Fact) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	for _, f := range facts {
		var args map[string]string
		require.NoError(t, json.Unmarshal([]byte(f.Args), &args))
		out[f.Relation] = args
	}
	return out
}

func TestComputeProjectsOwnerAlongBaseSchema(t *testing.T) {
	reg := NewRegistry(ownedSource(t), nil)
	web := reg.For("x.#Web")
	require.NotNil(t, web, "an inherited owner makes the schema project")
	require.NoError(t, web.Err)
	require.NotNil(t, web.Owner)

	labeled := ownerArgs(t, Compute(web, "e1", "r1", map[string]any{"labels": map[string]any{"team": "orders"}}).Facts)
	assert.Equal(t, map[string]string{"entry_id": "e1", "resource_id": "r1", "type": "x.#Web", "owner": "orders", "via": "path:labels.team"}, labeled[ownership.OwnerRelation])

	mapped := ownerArgs(t, Compute(web, "e2", "r2", map[string]any{"project": "proj-a"}).Facts)
	assert.Equal(t, "payments", mapped[ownership.OwnerRelation]["owner"])
	assert.Equal(t, "map:project", mapped[ownership.OwnerRelation]["via"])

	unowned := ownerArgs(t, Compute(web, "e3", "r3", map[string]any{"project": "proj-z"}).Facts)
	assert.NotContains(t, unowned, ownership.OwnerRelation)
	assert.Equal(t, map[string]string{"entry_id": "e3", "resource_id": "r3", "type": "x.#Web"}, unowned[ownership.UnownedRelation])

	args := reg.RelationArgs()
	assert.True(t, args[ownership.OwnerRelation]["owner"])
	assert.True(t, args[ownership.UnownedRelation]["type"])
	assert.ElementsMatch(t, []string{"x.#Host", "x.#Web"}, reg.RelationSchemas()[ownership.UnownedRelation])
}

func TestNearestOwnerWinsAndBrokenOwnerDisablesProjection(t *testing.T) {
	src := ownedSource(t)
	web := src["x.#Web"]
	web.Owner = ownerSpec(t, `{"fallback": "web-team"}`)
	src["x.#Web"] = web
	spec := NewRegistry(src, nil).For("x.#Web")
	owner, _, _ := spec.Owner.Resolve(map[string]any{"labels": map[string]any{"team": "orders"}})
	assert.Equal(t, "web-team", owner, "the nearer declaration replaces the parent's")

	host := src["x.#Host"]
	host.Owner, host.OwnerError = nil, "owner: unknown field"
	src["x.#Host"] = host
	web.Owner = nil
	src["x.#Web"] = web
	spec = NewRegistry(src, nil).For("x.#Web")
	require.NotNil(t, spec)
	require.Error(t, spec.Err)
	assert.Contains(t, spec.Err.Error(), "schema x.#Host: owner: unknown field")
}

func TestOwnerWithoutIdentityIsAnError(t *testing.T) {
	src := fakeSource{"x.#Host": validator.SchemaMetadata{SchemaType: "base", Owner: ownerSpec(t, `{"fallback": "ops"}`)}}
	spec := NewRegistry(src, nil).For("x.#Host")
	require.NotNil(t, spec)
	require.Error(t, spec.Err)
	assert.Contains(t, spec.Err.Error(), "declares an owner but no identity_fields")
}

// A changed mapping table changes the fingerprint, so Sync re-derives every
// resource's owner as a correction.
func TestSyncReprojectsOwnersWhenTheMappingChanges(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	addHostEntry(t, db, "e1", "rid1", `{"name":"h","project":"proj-a"}`)

	src := fakeSource{"x.#Host": {SchemaType: "base", IdentityFields: []string{"name"}, Owner: ownerSpec(t, `{"map": {"path": "project", "owners": {"proj-a": "payments"}}}`)}}
	_, err = Sync(context.Background(), db, NewRegistry(src, nil), loadJSON, false)
	require.NoError(t, err)
	owners, err := ownership.Load(db)
	require.NoError(t, err)
	assert.Equal(t, "payments", owners.OwnerOf([]string{"rid1"}, nil))
	assert.Equal(t, "payments", owners.OwnerOf(nil, []string{"e1"}))

	src["x.#Host"] = validator.SchemaMetadata{SchemaType: "base", IdentityFields: []string{"name"}, Owner: ownerSpec(t, `{"map": {"path": "project", "owners": {"proj-a": "search"}}}`)}
	report, err := Sync(context.Background(), db, NewRegistry(src, nil), loadJSON, false)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Reprojected)
	owners, err = ownership.Load(db)
	require.NoError(t, err)
	assert.Equal(t, "search", owners.OwnerOf([]string{"rid1"}, nil))
}

func TestFactsBlocksCannotDeclareOwnershipRelations(t *testing.T) {
	_, err := ParseRelations([]byte(`{"resource_owner": {"args": {"owner": "labels.team"}}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declare _pudl.owner instead")
}
