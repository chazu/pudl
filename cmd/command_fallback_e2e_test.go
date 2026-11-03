package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The search fails as if the asset API were disabled in the billing project;
// the fallback lists the firewall per project the way gcloud prints it.
const e2eFallbackModels = `package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#CaiOrGcloudFirewalls: sm.#SystemModel & {
	name: "fallback-firewalls"
	populate: {
		schema: "pudl/gcp.#Firewall"
		runs: [{
			argv: ["sh", "-c", "echo 'ERROR: (gcloud.asset.search-all-resources) reason: SERVICE_DISABLED' >&2; exit 1"]
			transform: {in: _, out: in.versionedResources[0].resource}
		}]
		fallback: {
			when: ["SERVICE_DISABLED"]
			observe: {
				each: {argv: ["printf", "%s", "[\"prod-a\",\"prod-b\"]"]}
				runs: [{
					argv: ["printf", "[{\"kind\":\"compute#firewall\",\"name\":\"allow-ssh\",\"direction\":\"INGRESS\",\"sourceRanges\":[\"0.0.0.0/0\"]}]%.0s", "{{item}}"]
					set: project: "{{item}}"
				}]
			}
		}
	}
	checks: [{name: "no-open-ingress", query: "open_ingress", expect: "empty", severity: "fail", message: "firewall open to the internet"}]
}
`

func TestCommandPopulateFallsBack(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue"), fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "gcp.cue"), strings.Split(e2eRules, "typo_rule")[0])
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "fallback.cue"), e2eFallbackModels)

	r := runCLI(t, "model", "validate", "fallback-firewalls")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "fallback-firewalls", "--json")
	require.Error(t, r.Err, "the fallback's open firewalls fail the check")
	report := checkedModel(t, r.Stdout)
	require.NotNil(t, report.Populate)
	require.NotNil(t, report.Populate.Fallback)
	assert.Equal(t, "failed_input", report.Populate.Fallback.Reason)
	assert.Equal(t, "SERVICE_DISABLED", report.Populate.Fallback.Pattern)
	assert.Equal(t, []string{"prod-a", "prod-b"}, report.Populate.Inputs)
	assert.Equal(t, 2, report.Populate.Records)
	assert.Empty(t, report.Populate.FailedInputs, "the search failure is not a failed input")
	require.Len(t, report.Checks, 1)
	assert.Equal(t, 2, report.Checks[0].Count)

	r = runCLI(t, "check", "fallback-firewalls")
	assert.Contains(t, r.Stdout, "source: fallback")
}
