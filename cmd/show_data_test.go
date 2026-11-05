package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importForShow imports content as one JSON entry and returns its ID.
func importForShow(t *testing.T, name, content string, args ...string) string {
	t.Helper()
	writeFile(t, name, content)
	r := runCLI(t, append([]string{"import", name, "--json"}, args...)...)
	require.NoError(t, r.Err, r.Stderr)
	var imported []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &imported), r.Stdout)
	require.Len(t, imported, 1)
	return imported[0].ID
}

func TestShowPrintsRecordByDefault(t *testing.T) {
	cliWorkspace(t)
	id := importForShow(t, "widget.json", `{"kind":"Widget","name":"w1","serial":12345678901234567890}`)

	r := runCLI(t, "show", id)
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "Hash: "+id+"\n")
	assert.Contains(t, r.Stdout, "\nRAW DATA\n")
	// Stored JSON is indented with its values and key order unchanged.
	assert.Contains(t, r.Stdout, "{\n  \"kind\": \"Widget\",\n  \"name\": \"w1\",\n  \"serial\": 12345678901234567890\n}\n")

	raw := runCLI(t, "show", id, "--raw")
	require.NoError(t, raw.Err, raw.Stderr)
	assert.Equal(t, r.Stdout, raw.Stdout, "--raw is the default")

	meta := runCLI(t, "show", id, "--meta-only")
	require.NoError(t, meta.Err, meta.Stderr)
	assert.Contains(t, meta.Stdout, "Hash: "+id+"\n")
	assert.NotContains(t, meta.Stdout, "RAW DATA")
	assert.NotContains(t, meta.Stdout, "w1")

	both := runCLI(t, "show", id, "--meta-only", "--metadata")
	require.NoError(t, both.Err, both.Stderr)
	assert.Contains(t, both.Stdout, "\nMETADATA\n")
	assert.NotContains(t, both.Stdout, "RAW DATA")

	for _, flag := range []string{"--raw", "--history"} {
		conflict := runCLI(t, "show", id, "--meta-only", flag)
		require.Error(t, conflict.Err, "--meta-only %s", flag)
	}
	conflict := runCLI(t, "show", id, "--meta-only", "--field", "name")
	require.Error(t, conflict.Err)
}

func TestShowJSONIncludesRecordUnlessMetaOnly(t *testing.T) {
	cliWorkspace(t)
	id := importForShow(t, "widget.json", `{"kind":"Widget","name":"w1"}`)
	show := func(args ...string) map[string]json.RawMessage {
		t.Helper()
		r := runCLI(t, append([]string{"show", id, "--json"}, args...)...)
		require.NoError(t, r.Err, r.Stderr)
		var out map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
		return out
	}

	def := show()
	require.JSONEq(t, `{"kind":"Widget","name":"w1"}`, string(def["raw"]))
	require.Contains(t, def, "entry")
	require.Contains(t, def, "compliance")
	require.NotContains(t, def, "metadata")
	require.Equal(t, def, show("--raw"))

	meta := show("--meta-only")
	require.NotContains(t, meta, "raw")
	require.NotContains(t, meta, "raw_text")
	require.Equal(t, def["entry"], meta["entry"])

	withMetadata := show("--meta-only", "--metadata")
	require.Contains(t, withMetadata, "metadata")
	require.NotContains(t, withMetadata, "raw")
}

func TestShowRecordKeepsSensitiveFieldsRedacted(t *testing.T) {
	const secret = "hunter2-correct-horse"
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "vault", "vault.cue"),
		fmt.Sprintf(scrubLoginSchema, `sensitive_fields: ["password"]`))
	reloadSchemas()
	id := importForShow(t, "login.json", `{"name":"svc","password":"`+secret+`"}`, "--schema", "pudl/vault.#Login")

	r := runCLI(t, "show", id)
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, `"name": "svc"`)
	assert.Contains(t, r.Stdout, `"password":`)
	assert.NotContains(t, r.Stdout, secret)

	r = runCLI(t, "show", id, "--json")
	require.NoError(t, r.Err, r.Stderr)
	assert.NotContains(t, r.Stdout, secret)
}
