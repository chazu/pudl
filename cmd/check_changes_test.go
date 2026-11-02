package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/checkreport"
)

// firewallInventory writes an inventory model whose one fail-severity check
// flags open ingress firewalls in its current snapshot, and returns a function
// that sets what the next observation sees.
func firewallInventory(t *testing.T, pudlDir string) func(complete bool, open ...string) {
	t.Helper()
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue"), fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "gcp.cue"), strings.Split(e2eRules, "typo_rule")[0])
	input := filepath.Join(t.TempDir(), "records.json")
	modelPath := filepath.Join(pudlDir, "schema", "models", "inventory.cue")
	return func(complete bool, open ...string) {
		writeFile(t, modelPath, fmt.Sprintf(`package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Inventory: sm.#SystemModel & {
 name:"inventory"
 observation:{scope:"gcp/prod/firewalls",complete:%t}
 populate:{schema:"pudl/gcp.#Firewall",runs:[{argv:["cat",%q],set:project:"prod"}]}
 checks:[{name:"closed",query:"open_ingress",expect:"empty",severity:"fail",message:"open firewall",evidence:["current"]}]
}`, complete, input))
		records := []string{`{"kind":"compute#firewall","name":"internal","direction":"INGRESS","sourceRanges":["10.0.0.0/8"]}`}
		for _, name := range open {
			records = append(records, fmt.Sprintf(`{"kind":"compute#firewall","name":%q,"direction":"INGRESS","sourceRanges":["0.0.0.0/0"]}`, name))
		}
		writeFile(t, input, "["+strings.Join(records, ",")+"]")
		reloadSchemas()
	}
}

// checkStep is one `pudl check inventory --json` and what it recorded.
type checkStep struct {
	id     string
	check  checkreport.CheckResult
	states map[string][]string
	seen   map[string]time.Time
}

func runCheckStep(t *testing.T, args ...string) (checkStep, cliResult) {
	t.Helper()
	r := runCLI(t, append([]string{"check", "inventory", "--json"}, args...)...)
	var top struct {
		ID string `json:"run_set_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &top), r.Stdout+r.Stderr)
	report := checkedModel(t, r.Stdout)
	require.Len(t, report.Checks, 1)
	step := checkStep{id: top.ID, check: report.Checks[0], states: map[string][]string{}, seen: map[string]time.Time{}}
	for _, f := range step.check.Findings {
		name, _ := f.Arguments["name"].(string)
		step.states[f.State] = append(step.states[f.State], name)
		require.NotNil(t, f.FirstSeen, "every finding records when it was first seen")
		step.seen[name] = *f.FirstSeen
	}
	for _, names := range step.states {
		sort.Strings(names)
	}
	return step, r
}

// TestCheckClassifiesFindingsOverTime runs a fixture sequence — baseline,
// regression, unchanged, partial observation, fix — and checks the new,
// persisting, resolved and unknown sets, first-seen times, --fail-on new and
// `report --since`.
func TestCheckClassifiesFindingsOverTime(t *testing.T) {
	pudlDir := cliWorkspace(t)
	observe := firewallInventory(t, pudlDir)

	observe(true, "a")
	baseline, r := runCheckStep(t, "--detailed-exitcode", "--fail-on", "new")
	assert.Equal(t, 2, exitCodeFor(r.Err), "the first report has nothing to compare with: every failure is new")
	assert.Equal(t, map[string][]string{"new": {"a"}}, baseline.states)

	observe(true, "a", "b")
	regression, r := runCheckStep(t, "--detailed-exitcode", "--fail-on", "new")
	assert.Equal(t, 2, exitCodeFor(r.Err))
	assert.Equal(t, map[string][]string{"new": {"b"}, "persisting": {"a"}}, regression.states)
	assert.Equal(t, baseline.seen["a"], regression.seen["a"], "a persisting finding keeps its first-seen time")
	assert.Equal(t, 1, regression.check.Changes.New)

	unchanged, r := runCheckStep(t, "--detailed-exitcode", "--fail-on", "new")
	assert.Equal(t, 0, exitCodeFor(r.Err), "--fail-on new passes when every failure was seen before: %s", r.Stderr)
	assert.Contains(t, r.Stderr, "--fail-on new")
	assert.Equal(t, map[string][]string{"persisting": {"a", "b"}}, unchanged.states)
	_, r = runCheckStep(t, "--fail-on", "new")
	assert.NoError(t, r.Err, "without --detailed-exitcode, known failures do not fail the command")
	_, r = runCheckStep(t, "--detailed-exitcode")
	assert.Equal(t, 2, exitCodeFor(r.Err), "--fail-on any still reports every failure")

	// A partial observation reaches no verdict: nothing is resolved.
	observe(false, "b")
	partial, r := runCheckStep(t, "--detailed-exitcode", "--fail-on", "new")
	assert.Equal(t, 1, exitCodeFor(r.Err))
	assert.Equal(t, "unknown", partial.check.Outcome)
	assert.Equal(t, map[string][]string{"unknown": {"a", "b"}}, partial.states)
	assert.Zero(t, partial.check.Changes.Resolved)

	// The fix: a complete observation without a.
	observe(true, "b")
	fixed, r := runCheckStep(t, "--detailed-exitcode", "--fail-on", "new")
	assert.Equal(t, 0, exitCodeFor(r.Err), r.Stderr)
	assert.Equal(t, map[string][]string{"persisting": {"b"}, "resolved": {"a"}}, fixed.states)
	assert.Equal(t, regression.seen["b"], fixed.seen["b"])
	assert.Equal(t, partial.check.Changes.Since, fixed.check.Changes.Since, "the partial report is not a baseline")

	human := runCLI(t, "report", fixed.id)
	require.NoError(t, human.Err)
	assert.Contains(t, human.Stdout, "0 new, 1 persisting, 1 resolved, 0 unknown")
	assert.Contains(t, human.Stdout, `resolved {"name":"a","project":"prod"}`)
	assert.Contains(t, human.Stdout, "findings: 0 new, 1 resolved")

	// Two named reports: the fix compared with the baseline.
	compared := runCLI(t, "report", fixed.id, "--since", baseline.id, "--json")
	require.NoError(t, compared.Err, compared.Stderr)
	var comparison checkreport.Comparison
	require.NoError(t, json.Unmarshal([]byte(compared.Stdout), &comparison))
	require.Len(t, comparison.Models, 1)
	require.Len(t, comparison.Models[0].Checks, 1)
	changes := comparison.Models[0].Checks[0].Changes
	assert.Equal(t, 1, changes.New)
	assert.Equal(t, 1, changes.Resolved)
	assert.Equal(t, 0, changes.Persisting)

	text := runCLI(t, "report", fixed.id, "--since", partial.id)
	require.NoError(t, text.Err)
	assert.Contains(t, text.Stdout, "not compared with")
	assert.Contains(t, text.Stdout, "reached no verdict")
}
