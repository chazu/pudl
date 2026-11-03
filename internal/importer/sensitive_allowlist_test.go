package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const allowlistSchema = `package gcp

_safeKeys: ["enable-oslogin", "vm-dns-setting"]

#VM: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.vm"
		identity_fields: ["name"]
		sensitive_fields: [{
			each:  "metadata.items[*]"
			value: "value"
			keep_when: {field: "key", in: _safeKeys}
		}]
	}
	kind: "compute#instance"
	name: string
	...
}
`

func TestImportAllowlistKeepsListedValuesAndRedactsTheRest(t *testing.T) {
	record := `{"kind":"compute#instance","name":"vm-1","metadata":{"items":[` +
		`{"key":"enable-oslogin","value":"TRUE"},` +
		`{"key":"startup-script","value":"curl -H token-s3cr3t"},` +
		`{"key":"vm-dns-setting","value":"ZonalOnly"},` +
		`{"key":"brand-new-key","value":"unreviewed-s3cr3t"}]}}` + "\n"
	imp, source := gcpFixture(t, allowlistSchema, "vm.ndjson", record)
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#VM"})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Redacted)

	items := storedItems(t, imp, result.ID)
	require.Len(t, items, 1)
	assert.NotContains(t, items[0], "s3cr3t", "unlisted keys, including new ones, are redacted")
	assert.Contains(t, items[0], `"TRUE"`)
	assert.Contains(t, items[0], `"ZonalOnly"`)
	assert.Contains(t, items[0], `"startup-script"`, "keys are kept")
}

func TestImportFailsClosedOnBrokenAllowlist(t *testing.T) {
	for name, entry := range map[string]string{
		"missing keep_when": `{each: "metadata.items[*]", value: "value"}`,
		"misspelled field":  `{each: "metadata.items[*]", value: "value", keep_when: {field: "key", in: []}, kep: 1}`,
		"each without [*]":  `{each: "metadata.items", value: "value", keep_when: {field: "key", in: []}}`,
		"missing in":        `{each: "metadata.items[*]", value: "value", keep_when: {field: "key"}}`,
		"not a struct":      `7`,
	} {
		t.Run(name, func(t *testing.T) {
			schema := `package gcp

#VM: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.vm"
		identity_fields: ["name"]
		sensitive_fields: [` + entry + `]
	}
	kind: "compute#instance"
	name: string
	...
}
`
			imp, source := gcpFixture(t, schema, "vm.ndjson", `{"kind":"compute#instance","name":"a","metadata":{"items":[{"key":"k","value":"v"}]}}`+"\n")
			_, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#VM"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "sensitive_fields")
		})
	}
}
