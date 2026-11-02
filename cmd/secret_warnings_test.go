package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cliTestToken is token-shaped test data built at run time.
func cliTestToken() string { return "ghp_" + strings.Repeat("aB3dE5fG7hJ9", 3) }

func TestImportReportsLikelySecretsWithoutValues(t *testing.T) {
	cliWorkspace(t)
	token := cliTestToken()
	require.NoError(t, os.WriteFile("misc.ndjson", []byte(`{"misc":"a","auth":{"token":"`+token+`"}}`+"\n"), 0o644))

	preview := runCLI(t, "import", "--path", "misc.ndjson", "--dry-run")
	require.NoError(t, preview.Err, preview.Stderr)
	assert.Contains(t, preview.Stdout, "1 likely secrets outside the schema's sensitive fields")
	assert.Contains(t, preview.Stdout, "record 0 (")
	assert.Contains(t, preview.Stdout, "auth.token (github_token)")
	assert.Contains(t, preview.Stdout, "_pudl.sensitive_fields")
	assert.Contains(t, preview.Stdout, "_pudl.secret_scan_ignore")

	r := runCLI(t, "import", "--path", "misc.ndjson", "--json")
	require.NoError(t, r.Err, r.Stderr)
	var outcomes []struct {
		SecretWarnings []struct {
			Record int    `json:"record"`
			Schema string `json:"schema"`
			Path   string `json:"path"`
			Kind   string `json:"kind"`
		} `json:"secret_warnings"`
		SecretWarningsTotal int `json:"secret_warnings_total"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &outcomes), r.Stdout)
	require.Len(t, outcomes, 1)
	require.Equal(t, 1, outcomes[0].SecretWarningsTotal)
	assert.Equal(t, "auth.token", outcomes[0].SecretWarnings[0].Path)
	assert.Equal(t, "github_token", outcomes[0].SecretWarnings[0].Kind)
	for _, out := range []string{preview.Stdout, preview.Stderr, r.Stdout, r.Stderr} {
		assert.NotContains(t, out, token[4:20], "no output carries the value")
	}
}

func TestFactsAddWarnsOnLikelySecrets(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "facts", "add", "--relation", "credential", "--args", `{"token":"`+cliTestToken()+`"}`, "--json")
	require.NoError(t, r.Err, r.Stderr)
	assert.True(t, json.Valid([]byte(r.Stdout)), "the warning stays off stdout")
	assert.Contains(t, r.Stderr, "args contain likely secrets: token (github_token)")
	assert.NotContains(t, r.Stderr, cliTestToken()[4:20])

	clean := runCLI(t, "facts", "add", "--relation", "host", "--args", `{"name":"h1"}`)
	require.NoError(t, clean.Err)
	assert.NotContains(t, clean.Stderr, "likely secrets")
}
