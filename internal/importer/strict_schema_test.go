package importer

import (
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/require"
)

func TestExplicitSchemaRejectsAtomicallyAndPreviewAgrees(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"item.json", `{"name":"invalid"}`},
		{"items.ndjson", "{\"apiVersion\":\"v1\",\"kind\":\"Pod\",\"metadata\":{\"name\":\"ok\"}}\n{\"name\":\"invalid\"}\n"},
		{"items.json", `[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"ok"}},{"name":"invalid"}]`},
		{"item.yaml", "name: invalid\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imp, source, _ := collectionFixture(t, tc.name, tc.data)
			opts := ImportOptions{SourcePath: source, ManualSchema: "pudl/k8s.#Resource"}
			_, err := imp.Preview(opts)
			require.ErrorContains(t, err, "does not satisfy requested schema")
			_, err = imp.ImportFileWithFriendlyIDs(opts)
			require.ErrorContains(t, err, "does not satisfy requested schema")
			entries, err := imp.catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
			require.NoError(t, err)
			require.Empty(t, entries.Entries, "a bad collection record must not publish earlier records")
			opts.AllowSchemaFallback = true
			preview, err := imp.Preview(opts)
			require.NoError(t, err)
			require.Positive(t, preview.SchemaMismatches)
			result, err := imp.ImportFileWithFriendlyIDs(opts)
			require.NoError(t, err)
			require.Equal(t, "permissive", result.SchemaPolicy)
			require.Positive(t, result.SchemaMismatches)
			opts.AllowSchemaFallback = false
			_, err = imp.ImportFileWithFriendlyIDs(opts)
			require.ErrorContains(t, err, "does not satisfy requested schema", "dedup cannot bypass validation")
		})
	}
}

const bucketPolicySchemas = `package gcp

#Bucket: {
	_pudl: {schema_type: "base", resource_type: "gcp.bucket", identity_fields: ["name"]}
	name: string
	...
}

#BucketHardened: {
	_pudl: {schema_type: "policy", base_schema: "pudl/gcp.#Bucket"}
	encrypted: true
	...
}
`

// TestExplicitPolicySchemaIsRejected covers --schema naming a policy: preview
// and import fail up front, fallback does not excuse it, the error names the
// policy's type and policy evaluation, and inference assigns the type.
func TestExplicitPolicySchemaIsRejected(t *testing.T) {
	imp, source := gcpFixture(t, bucketPolicySchemas, "bucket.json", `{"name":"b","encrypted":true}`)
	for _, fallback := range []bool{false, true} {
		opts := ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#BucketHardened", AllowSchemaFallback: fallback}
		_, err := imp.Preview(opts)
		require.ErrorContains(t, err, "name its type pudl/gcp.#Bucket instead")
		_, err = imp.ImportFileWithFriendlyIDs(opts)
		require.ErrorContains(t, err, "pudl/gcp.#BucketHardened is a policy schema")
		require.ErrorContains(t, err, "pudl policy evaluate")
	}
	entries, err := imp.catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	require.Empty(t, entries.Entries)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	require.Equal(t, "pudl/gcp.#Bucket", result.AssignedSchema, "inference assigns the type, never the policy")
}
