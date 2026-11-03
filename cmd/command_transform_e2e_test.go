package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A CAI-shaped search result: the firewall sits in versionedResources, and a
// second asset type comes back in the same output.
const e2eTransformModels = `package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#CaiFirewalls: sm.#SystemModel & {
	name: "cai-firewalls"
	populate: {
		schema: "pudl/gcp.#Firewall"
		runs: [{
			argv: ["printf", "%s", "[{\"assetType\":\"compute.googleapis.com/Firewall\",\"versionedResources\":[{\"resource\":{\"kind\":\"compute#firewall\",\"name\":\"allow-ssh\",\"direction\":\"INGRESS\",\"sourceRanges\":[\"0.0.0.0/0\"]}}]},{\"assetType\":\"compute.googleapis.com/Route\",\"versionedResources\":[{\"resource\":{\"name\":\"r\"}}]}]"]
			set: project: "prod-a"
			transform: {
				in: _
				out: [if in.assetType != "compute.googleapis.com/Firewall" {null}, in.versionedResources[0].resource][0]
			}
		}]
	}
	checks: [{name: "no-open-ingress", query: "open_ingress", expect: "empty", severity: "fail", message: "firewall open to the internet"}]
}
`

func TestCommandPopulateTransformsRecords(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue"), fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "gcp.cue"), strings.Split(e2eRules, "typo_rule")[0])
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "cai.cue"), e2eTransformModels)

	r := runCLI(t, "model", "validate", "cai-firewalls")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "cai-firewalls", "--json")
	require.Error(t, r.Err, "the unwrapped open firewall fails the check")
	report := checkedModel(t, r.Stdout)
	require.NotNil(t, report.Populate)
	assert.Equal(t, 1, report.Populate.Records, "the route was dropped by the transform")
	assert.Empty(t, report.Populate.FailedInputs)
	require.Len(t, report.Checks, 1)
	assert.Equal(t, 1, report.Checks[0].Count)
}
