package resourceref

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/validator"
)

func addEntry(t *testing.T, db *database.CatalogDB, id, rid, schema, identity string) {
	t.Helper()
	require.NoError(t, db.AddEntry(database.CatalogEntry{
		ID: id, StoredPath: t.TempDir() + "/" + id + ".json", MetadataPath: t.TempDir() + "/m", ImportTimestamp: time.Now(),
		Format: "json", Origin: "t", Schema: schema, Confidence: 1, ResourceID: &rid, ContentHash: &id, IdentityJSON: &identity,
	}))
}

func TestResolveAndDescribe(t *testing.T) {
	db, err := database.NewCatalogDB(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	addEntry(t, db, "e1", "rid-key", "gcp.#ServiceAccountKey", `{"name":"projects/p1/serviceAccounts/sa@p1/keys/k1"}`)
	addEntry(t, db, "e2", "rid-a", "x.#A", `{"name":"shared"}`)
	addEntry(t, db, "e3", "rid-b", "x.#B", `{"name":"shared"}`)
	addEntry(t, db, "e4", "rid-multi", "x.#M", `{"project":"p1","name":"m"}`)
	addEntry(t, db, "e5", "rid-sa", "gcp.#ServiceAccount", `{"email":"sa@p1"}`)

	graph, err := graphspec.Parse([]byte(`{"account": "project"}`))
	require.NoError(t, err)
	meta := func(schema string) (validator.SchemaMetadata, bool) {
		if schema == "gcp.#ServiceAccountKey" {
			return validator.SchemaMetadata{ResourceType: "gcp.iam.service_account_key", Graph: graph}, true
		}
		return validator.SchemaMetadata{}, true
	}
	loads := 0
	load := func(e database.CatalogEntry) (any, error) {
		loads++
		return map[string]any{"project": "p1"}, nil
	}
	x, err := Load(db, meta, load)
	require.NoError(t, err)

	id, value, ok := x.Resolve(map[string]any{"account": "sa@p1", "key": "projects/p1/serviceAccounts/sa@p1/keys/k1"})
	require.True(t, ok, "any argument may name the resource")
	// Both name a resource; the key's name contains the account's email, so
	// the key is the more specific one.
	assert.Equal(t, "rid-key", id)
	assert.Equal(t, "projects/p1/serviceAccounts/sa@p1/keys/k1", value)

	id, _, ok = x.Resolve(map[string]any{"resource_id": "rid-multi"})
	assert.True(t, ok)
	assert.Equal(t, "rid-multi", id, "a resource ID names itself")

	id, _, _ = x.Resolve(map[string]any{"account": "sa@p1"})
	assert.Equal(t, "rid-sa", id)

	_, _, ok = x.Resolve(map[string]any{"name": "shared"})
	assert.False(t, ok, "a value two resources share names neither")
	_, _, ok = x.Resolve(map[string]any{"name": "m"})
	assert.False(t, ok, "a multi-field identity is not one value")

	d := x.Describe("rid-key")
	assert.Equal(t, "p1", d.Account)
	assert.Equal(t, "gcp.iam.service_account_key", d.Type)
	assert.Equal(t, "projects/p1/serviceAccounts/sa@p1/keys/k1", d.Name, "no graph or record name: the identity value")
	x.Describe("rid-key")
	assert.Equal(t, 1, loads, "descriptions are cached")
}
