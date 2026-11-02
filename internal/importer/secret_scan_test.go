package importer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/secrets"
)

// testToken builds a token-shaped value at run time: prefix plus n mixed
// characters.
func testToken(prefix string, n int) string {
	const alphabet = "aB3dE5fG7hJ9kL1mN2pQ4rS6tU8vW0xYz"
	var b strings.Builder
	b.WriteString(prefix)
	for i := 0; i < n; i++ {
		b.WriteByte(alphabet[(i*7+i/len(alphabet))%len(alphabet)])
	}
	return b.String()
}

const scanSchema = `package gcp

#Base: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.scanned"
		identity_fields: ["name"]
		secret_scan_ignore: ["spec.nonce"]
	}
	kind: "scanned"
	name: string
	...
}

#Child: {
	_pudl: {
		schema_type:   "custom"
		resource_type: "gcp.scanned"
		base_schema:   "pudl/gcp.#Base"
		identity_fields: ["name"]
		sensitive_fields: ["spec.token"]
	}
	kind: "scanned"
	name: string
	...
}
`

func TestImportWarnsOnCatchallSecretWithoutChangingStorage(t *testing.T) {
	token := testToken("ghp_", 36)
	content := `{"misc":"a"}` + "\n" + `{"misc":"b","auth":{"token":"` + token + `"}}` + "\n"
	imp, source := gcpFixture(t, firewallSchema, "misc.ndjson", content)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	require.Equal(t, 1, result.SecretWarningsTotal)
	w := result.SecretWarnings[0]
	assert.Equal(t, 1, w.Record)
	assert.Equal(t, secrets.Finding{Path: "auth.token", Kind: secrets.GitHubToken}, w.Finding)
	assert.NotEmpty(t, w.Schema)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), token[4:20], "the warning never carries the value")
	assert.Contains(t, string(encoded), `"secret_warnings":[{"record":1,`)

	stored, err := os.ReadFile(result.StoredPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(stored), "the scan never rewrites stored data")
	f, err := os.Open(source)
	require.NoError(t, err)
	defer f.Close()
	hash, err := idgen.ComputeContentIDReader(f)
	require.NoError(t, err)
	assert.Equal(t, hash, result.ContentHash, "the scan never changes the content hash")
}

func TestImportWarnsOnDocumentSecret(t *testing.T) {
	content := `{"settings":{"dsn":"postgres://app:` + testToken("", 12) + `@db:5432/app"}}`
	imp, source := gcpFixture(t, firewallSchema, "settings.json", content)
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	require.Len(t, result.SecretWarnings, 1)
	assert.Equal(t, SecretWarning{Record: 0, Schema: result.AssignedSchema,
		Finding: secrets.Finding{Path: "settings.dsn", Kind: secrets.URLCredentials}}, result.SecretWarnings[0])
}

func TestImportSecretScanHonorsSchemaCoverage(t *testing.T) {
	// spec.token is sensitive (redacted) on the child; spec.nonce is declared
	// not secret on the base and inherited. Only spec.other is reported.
	record := `{"kind":"scanned","name":"a","spec":{"token":"` + testToken("ghp_", 36) +
		`","nonce":"` + testToken("", 40) + `","other":"` + testToken("glpat-", 24) + `"}}` + "\n"
	imp, source := gcpFixture(t, scanSchema, "scanned.ndjson", record)

	preview, err := imp.Preview(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#Child"})
	require.NoError(t, err)
	require.Equal(t, 1, preview.SecretWarningsTotal, "%+v", preview.SecretWarnings)
	assert.Equal(t, secrets.Finding{Path: "spec.other", Kind: secrets.GitLabToken}, preview.SecretWarnings[0].Finding)

	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: source, ManualSchema: "pudl/gcp.#Child"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Redacted)
	assert.Equal(t, preview.SecretWarnings, result.SecretWarnings, "dry run and import agree")
}

func TestImportPreviewCapsSecretWarnings(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxSecretWarnings+5; i++ {
		b.WriteString(`{"n":` + strings.Repeat("1", i%3+1) + `,"k":"` + testToken("ghp_", 36) + `"}` + "\n")
	}
	imp, source := gcpFixture(t, firewallSchema, "many.ndjson", b.String())
	result, err := imp.Preview(ImportOptions{SourcePath: source})
	require.NoError(t, err)
	assert.Equal(t, maxSecretWarnings+5, result.SecretWarningsTotal)
	assert.Len(t, result.SecretWarnings, maxSecretWarnings)
}
