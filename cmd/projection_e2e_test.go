package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const e2eFirewallBase = `package gcp

#Firewall: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.firewall"
		identity_fields: ["project", "name"]
%s
	}
	kind: "compute#firewall"
	...
}
`

const e2eFirewallFacts = `		facts: {
			gcp_firewall: args: {project: "project", name: "name", direction: "direction", disabled: {path: "disabled", default: false}}
			gcp_firewall_source: args: {range: "sourceRanges[*]"}
		}`

const e2eRules = `package rules

open_ingress: {
	head: {rel: "open_ingress", args: {project: "$P", name: "$N"}}
	body: [
		{rel: "gcp_firewall", args: {entry_id: "$E", project: "$P", name: "$N", direction: "INGRESS", disabled: false}},
		{rel: "gcp_firewall_source", args: {entry_id: "$E", range: "0.0.0.0/0"}},
	]
}

typo_rule: {
	head: {rel: "typo_rule", args: {name: "$N"}}
	body: [{rel: "gcp_firewall", args: {nmae: "$N"}}]
}
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func reloadSchemas() {
	inference.ResetShared()
	validator.ResetSharedLoaders()
}

func TestImportedFieldsReachQueriesWithoutJQ(t *testing.T) {
	pudlDir := cliWorkspace(t)
	schemaPath := filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue")
	// The schema starts without a facts block: data imported before one is
	// declared must still become queryable once it is.
	writeFile(t, schemaPath, fmt.Sprintf(e2eFirewallBase, ""))
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "gcp.cue"), e2eRules)
	writeFile(t, "fw-prod-a.json", `[
		{"kind":"compute#firewall","name":"allow-ssh","direction":"INGRESS","sourceRanges":["0.0.0.0/0"]},
		{"kind":"compute#firewall","name":"internal","direction":"INGRESS","sourceRanges":["10.0.0.0/8"]}
	]`)

	r := runCLI(t, "import", "fw-prod-a.json", "--schema", "pudl/gcp.#Firewall", "--set", "project=prod-a")
	require.NoError(t, r.Err, r.Stderr)

	writeFile(t, schemaPath, fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	reloadSchemas()

	r = runCLI(t, "query", "open_ingress")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stderr, "never been projected", "a pending projection is announced, not silent")

	r = runCLI(t, "facts", "reproject")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "2 bootstrapped")

	r = runCLI(t, "query", "open_ingress")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, `"name":"allow-ssh"`)
	assert.NotContains(t, r.Stdout, `"name":"internal"`)
	assert.Contains(t, r.Stdout, `"project":"prod-a"`, "--set supplied the project gcloud omits")

	r = runCLI(t, "query", "gcp_firewall", "disabled=false")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "allow-ssh", "a typed boolean constraint matches projected booleans")

	r = runCLI(t, "query", "typo_rule")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stderr, `relation gcp_firewall has no arg "nmae"`)
}
