package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const e2eCommandModels = `package models

import sm "pudl.schemas/pudl/systemmodel@v0"

_projects: ["prod-a", "prod-b"]

#GcpFirewalls: sm.#SystemModel & {
	name: "gcp-firewalls"
	populate: {
		schema: "pudl/gcp.#Firewall"
		runs: [for p in _projects {
			argv: ["printf", "%s", "[{\"kind\":\"compute#firewall\",\"name\":\"allow-ssh\",\"direction\":\"INGRESS\",\"sourceRanges\":[\"0.0.0.0/0\"]}]"]
			set: project: p
		}]
	}
	checks: [{name: "no-open-ingress", query: "open_ingress", expect: "empty", severity: "fail", message: "firewall open to the internet"}]
}

#GcpPosture: sm.#SystemModel & {
	name: "gcp-posture"
	checks: [{name: "no-open-ingress", query: "open_ingress", expect: "empty", severity: "fail", message: "firewall open to the internet"}]
}
`

func TestCommandPopulateAndChecksOnlyModels(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue"), fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "gcp.cue"), strings.Split(e2eRules, "typo_rule")[0])
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp.cue"), e2eCommandModels)

	r := runCLI(t, "model", "validate", "gcp-firewalls")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "run", "gcp-firewalls", "--json")
	require.Error(t, r.Err, "the open firewall fails the check")
	var report RunReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	require.NotNil(t, report.Populate)
	assert.Equal(t, 2, report.Populate.Records, "one record per project run")
	require.Len(t, report.Checks, 1)
	assert.False(t, report.Checks[0].Passed)
	assert.Equal(t, 2, report.Checks[0].Count, "set: project kept the two projects' firewalls distinct")

	// A checks-only model evaluates the same check over the catalog as is.
	r = runCLI(t, "run", "gcp-posture", "--json")
	require.Error(t, r.Err)
	report = RunReport{}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	assert.Equal(t, "checks-only", report.Mode)
	assert.Nil(t, report.Populate)
	require.Len(t, report.Checks, 1)
	assert.Equal(t, 2, report.Checks[0].Count)
}
