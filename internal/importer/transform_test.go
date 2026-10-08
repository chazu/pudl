package importer

import (
	"os"
	"strings"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const firewallSchema = `package gcp

#Firewall: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.firewall"
		identity_fields: ["project", "name"]
	}
	kind: "compute#firewall"
	name: string
	...
}

#RunService: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.run.service"
		identity_fields: ["name"]
		sensitive_fields: ["env[*].value"]
	}
	kind: "run#service"
	name: string
	env?: [...{name: string, value?: string}]
	...
}
`

func set(t *testing.T, assignments ...string) []FieldAssignment {
	var out []FieldAssignment
	for _, a := range assignments {
		k, v, _ := strings.Cut(a, "=")
		out = append(out, FieldAssignment{Path: fieldpath.MustParse(k), Value: v})
	}
	return out
}

func storedItems(t *testing.T, imp *EnhancedImporter, collectionID string) []string {
	items, err := imp.catalogDB.GetCollectionItems(collectionID)
	require.NoError(t, err)
	var out []string
	for _, item := range items {
		b, err := os.ReadFile(item.StoredPath)
		require.NoError(t, err)
		out = append(out, string(b))
	}
	return out
}

func TestImportSetInjectsFieldAndFormsIdentity(t *testing.T) {
	imp, source := gcpFixture(t, firewallSchema, "fw.json", `[{"kind":"compute#firewall","name":"allow-ssh","n":12345678901234567890}]`)
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#Firewall", Set: set(t, "project=prod-a")})
	require.NoError(t, err)
	assert.Equal(t, 0, result.IdentityUnresolved)
	assert.Equal(t, "json", result.DetectedFormat)
	items := storedItems(t, imp, result.ID)
	require.Len(t, items, 1)
	assert.Contains(t, items[0], `"project": "prod-a"`)
	assert.Contains(t, items[0], "12345678901234567890", "numbers survive the rewrite exactly")

	// The same source with the same --set deduplicates.
	again, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#Firewall", Set: set(t, "project=prod-a")})
	require.NoError(t, err)
	assert.True(t, again.Skipped)
}

func TestImportRedactsSensitiveFieldsBeforeStorage(t *testing.T) {
	records := `{"kind":"run#service","name":"api","env":[{"name":"TOKEN","value":"s3cr3t"},{"name":"MODE"}]}
`
	imp, source := gcpFixture(t, firewallSchema, "run.ndjson", records)
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#RunService"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Redacted)

	items := storedItems(t, imp, result.ID)
	require.Len(t, items, 1)
	assert.NotContains(t, items[0], "s3cr3t")
	assert.Contains(t, items[0], `"[REDACTED]"`)
	collection, err := os.ReadFile(result.StoredPath)
	require.NoError(t, err)
	assert.NotContains(t, string(collection), "s3cr3t", "the collection's own stored file is redacted too")

	entries, err := imp.catalogDB.GetCollectionItems(result.ID)
	require.NoError(t, err)
	assert.Equal(t, "pudl/gcp.#RunService", entries[0].Schema, "classification reflects the original record")
}

func TestImportUnchangedSourceKeepsItsHash(t *testing.T) {
	// A sensitive schema is loaded, so the pre-pass runs, but this source has
	// nothing to redact: it must import byte-for-byte unchanged.
	content := `{"kind":"compute#firewall","name":"a"}` + "\n"
	imp, source := gcpFixture(t, firewallSchema, "fw.ndjson", content)
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	stored, err := os.ReadFile(result.StoredPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(stored))
	assert.Equal(t, 0, result.Redacted)
}

func TestImportFailsClosedOnBrokenSensitiveDeclaration(t *testing.T) {
	schema := `package gcp

#Secretish: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.secretish"
		identity_fields: ["name"]
		sensitive_fields: ["value"]
		sensitive_mode: "hash"
	}
	kind: "secretish"
	...
}
`
	imp, source := gcpFixture(t, schema, "s.ndjson", `{"kind":"secretish","name":"a","value":"x"}`+"\n")
	_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#Secretish"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sensitive_mode")
}

func TestImportSetRejectsNonJSONDocuments(t *testing.T) {
	imp, source := gcpFixture(t, firewallSchema, "x.yaml", "name: a\n")
	_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, Set: set(t, "project=p")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--set applies only to JSON")
}

func TestReimportWithSchemaReassignsExistingItems(t *testing.T) {
	content := `{"kind":"compute#firewall","project":"p","name":"a"}` + "\n"
	imp, source := gcpFixture(t, firewallSchema, "fw.ndjson", content)

	first, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/core.#Item"})
	require.NoError(t, err)
	items, err := imp.catalogDB.GetCollectionItems(first.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotEqual(t, "pudl/gcp.#Firewall", items[0].Schema)

	// Same file again with the corrected schema: whole-file dedup, items move.
	second, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#Firewall"})
	require.NoError(t, err)
	assert.True(t, second.Skipped)
	assert.Equal(t, 1, second.Reassigned)

	moved, err := imp.catalogDB.GetEntry(items[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "pudl/gcp.#Firewall", moved.Schema)
	require.NotNil(t, moved.IdentityJSON)
	assert.JSONEq(t, `{"project":"p","name":"a"}`, *moved.IdentityJSON)
	require.NotNil(t, moved.Version)
	assert.Equal(t, 1, *moved.Version)

	// A schema the record does not satisfy moves nothing.
	third, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#RunService"})
	require.NoError(t, err)
	assert.Equal(t, 0, third.Reassigned)
}

func TestPreviewWritesNothingAndReportsFindings(t *testing.T) {
	records := `{"kind":"run#service","name":"api","env":[{"name":"TOKEN","value":"s3cr3t"}]}
{"kind":"run#service","name":5}
{"kind":"run#service","env":[]}
`
	imp, source := gcpFixture(t, firewallSchema, "run.ndjson", records)
	result, err := imp.Preview(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#RunService"})
	require.NoError(t, err)
	require.True(t, result.DryRun)
	assert.Equal(t, 3, result.RecordCount)
	assert.Equal(t, 1, result.Redacted)
	assert.Equal(t, 2, result.Preview.ValidationFailures, "name: 5 is not a string; the third record has no name")
	assert.NotEmpty(t, result.Preview.ValidationIssues)
	assert.Equal(t, 3, result.Preview.Schemas["pudl/gcp.#RunService"]+result.Preview.Schemas["pudl/core.#Item"])

	all, err := imp.catalogDB.QueryEntries(database.FilterOptions{}, database.QueryOptions{})
	require.NoError(t, err)
	assert.Empty(t, all.Entries, "a dry run writes no catalog rows")
}
