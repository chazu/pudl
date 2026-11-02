package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fleet fixture: servers owned by a label, by their project's team, and
// by nobody, plus a specialized type that inherits ownership along
// base_schema.
const ownershipSchema = `package fleet

_projectOwners: {"proj-a": "payments"}

#Server: {
	_pudl: {
		schema_type:   "base"
		resource_type: "fleet.server"
		identity_fields: ["name"]
		owner: {paths: ["labels.team"], map: {path: "project", owners: _projectOwners}}
		facts: fleet_server: args: {name: "name", public: {path: "public", default: false}}
	}
	name:    string
	project: string
	...
}

#GPUServer: {
	_pudl: {
		schema_type:   "base"
		resource_type: "fleet.gpu"
		base_schema:   "pudl/fleet.#Server"
		identity_fields: ["name"]
	}
	name:    string
	project: string
	gpu:     string
	...
}

#ServerBestPractice: {
	_pudl: {
		schema_type: "policy"
		base_schema: "pudl/fleet.#Server"
		identity_fields: ["name"]
	}
	public?: false @pudl(rule="fleet-no-public", severity="high")
}
`

const ownershipRules = `package rules

public_server: {
	head: {rel: "public_server", args: {resource: "$R", name: "$N"}}
	body: [{rel: "fleet_server", args: {resource_id: "$R", name: "$N", public: true}}]
}
`

const ownershipModel = `package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#Fleet: sm.#SystemModel & {
	name: "fleet"
	checks: [
		{name: "public", query: "public_server", expect: "empty", severity: "fail", message: "public servers"},
		{name: "unowned", query: "resource_unowned", expect: "empty", severity: "warn", message: "resources without an owner"},
	]
}
`

func ownershipWorkspace(t *testing.T) {
	t.Helper()
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "fleet", "fleet.cue"), ownershipSchema)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "fleet.cue"), ownershipRules)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "fleet.cue"), ownershipModel)
	writeFile(t, "servers.json", `[
		{"name": "web-1", "project": "proj-a", "public": true, "labels": {"team": "orders"}},
		{"name": "web-2", "project": "proj-a", "public": true},
		{"name": "web-3", "project": "proj-z", "public": true},
		{"name": "db-1", "project": "proj-a", "public": false}
	]`)
	writeFile(t, "gpus.json", `[{"name": "gpu-1", "project": "proj-z", "gpu": "a100", "public": true, "labels": {"team": "ml"}}]`)
	for file, schema := range map[string]string{"servers.json": "pudl/fleet.#Server", "gpus.json": "pudl/fleet.#GPUServer"} {
		r := runCLI(t, "import", file, "--schema", schema)
		require.NoError(t, r.Err, r.Stdout+r.Stderr)
	}
}

// findingOwners maps each witness's name (or type, for unowned resources) to
// its owner, per check.
func findingOwners(t *testing.T, stdout string) map[string]map[string]string {
	t.Helper()
	report := checkedModel(t, stdout)
	out := map[string]map[string]string{}
	for _, c := range report.Checks {
		owners := map[string]string{}
		for _, w := range c.Witnesses {
			key, _ := w.Arguments["name"].(string)
			if key == "" {
				key, _ = w.Arguments["type"].(string)
			}
			owners[key] = w.Owner
		}
		out[c.Name] = owners
	}
	return out
}

func TestOwnershipAttributesFindingsAndListsUnowned(t *testing.T) {
	ownershipWorkspace(t)

	r := runCLI(t, "check", "fleet", "--json", "--detailed-exitcode")
	require.Equal(t, 2, exitCodeFor(r.Err), r.Stdout+r.Stderr)
	owners := findingOwners(t, r.Stdout)
	assert.Equal(t, map[string]string{"web-1": "orders", "web-2": "payments", "web-3": "", "gpu-1": "ml"}, owners["public"],
		"label first, then the project mapping; the GPU type inherits the declaration")
	assert.Equal(t, map[string]string{"pudl/fleet.#Server": ""}, owners["unowned"], "web-3 is the one unowned resource")
	assert.Contains(t, r.Stdout, `"owner": "payments"`)

	r = runCLI(t, "check", "fleet") // fails: a fail-severity check found public servers
	require.Contains(t, r.Stdout, "unowned [warn]", r.Stdout+r.Stderr)
	public := r.Stdout[strings.Index(r.Stdout, "public [fail]"):strings.Index(r.Stdout, "unowned [warn]")]
	assert.Regexp(t, `(?s)- owner ml:\n.*gpu-1.*- owner orders:\n.*web-1.*- owner payments:\n.*web-2.*- owner \(none\):\n.*web-3`, public, "human output groups findings by owner")

	r = runCLI(t, "check", "fleet", "--owner", "payments", "--json", "--detailed-exitcode")
	require.Equal(t, 2, exitCodeFor(r.Err), "the exit status still covers every finding")
	assert.Equal(t, map[string]map[string]string{"public": {"web-2": "payments"}}, findingOwners(t, r.Stdout),
		"only payments' findings; the unowned check has none and is left out")

	r = runCLI(t, "check", "fleet", "--owner", "payments")
	assert.Contains(t, r.Stdout, "web-2", r.Stdout+r.Stderr)
	assert.NotContains(t, r.Stdout, "web-1")
	assert.NotContains(t, r.Stdout, "unowned [warn]")

	stored := runCLI(t, "report", "--json")
	require.NoError(t, stored.Err)
	assert.Contains(t, stored.Stdout, "web-1", "the recorded report keeps every finding")

	r = runCLI(t, "query", "resource_unowned", "--json")
	require.NoError(t, r.Err, r.Stderr)
	assert.Equal(t, 1, strings.Count(r.Stdout, `"resource_id"`), r.Stdout)
	r = runCLI(t, "query", "resource_owner", "--json")
	require.NoError(t, r.Err, r.Stderr)
	for _, via := range []string{`"via":"path:labels.team"`, `"via":"map:project"`} {
		assert.Contains(t, strings.ReplaceAll(r.Stdout, " ", ""), via)
	}
}

func TestPolicyShowAttributesFailingResources(t *testing.T) {
	ownershipWorkspace(t)
	r := runCLI(t, "policy", "evaluate", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	var shown struct {
		Failing []struct {
			EntryID string `json:"entry_id"`
			Owner   string `json:"owner"`
		} `json:"failing"`
		FailingTotal int `json:"failing_total"`
	}
	r = runCLI(t, "policy", "show", "pudl/fleet.#ServerBestPractice", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &shown), r.Stdout)
	var owners []string
	for _, f := range shown.Failing {
		owners = append(owners, f.Owner)
	}
	assert.ElementsMatch(t, []string{"orders", "payments", "", "ml"}, owners, r.Stdout)

	r = runCLI(t, "policy", "show", "pudl/fleet.#ServerBestPractice", "--owner", "payments", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &shown))
	require.Equal(t, 1, shown.FailingTotal)
	assert.Equal(t, "payments", shown.Failing[0].Owner)
}

func TestInvalidOwnerMetadataFailsToLoad(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "fleet", "fleet.cue"),
		strings.Replace(ownershipSchema, `owner: {paths: ["labels.team"]`, `owner: {pathz: ["labels.team"]`, 1))
	r := runCLI(t, "doctor", "--json")
	assert.Contains(t, r.Stdout, "has invalid _pudl.owner", r.Stdout+r.Stderr)
	assert.Contains(t, r.Stdout, `unknown field \"pathz\"`)
}

func TestGCPPackDeclaresOwnership(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "example", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "check", "gcp-fixture-pubsub-topics", "gcp-fixture-buckets", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "query", "resource_owner", "--json")
	require.NoError(t, r.Err, r.Stderr)
	owned := strings.ReplaceAll(r.Stdout, " ", "")
	assert.Equal(t, 2, strings.Count(owned, `"owner":"orders"`), "both fixture topics carry labels.team: orders")
	assert.Contains(t, owned, `"via":"path:labels.team"`)

	r = runCLI(t, "query", "resource_unowned", "--json")
	require.NoError(t, r.Err, r.Stderr)
	assert.Contains(t, r.Stdout, "gcp.#Bucket", "unlabeled buckets in an unmapped project are unowned")
}
