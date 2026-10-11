package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotChecksRespectCompletePopulationAndKeepHistory(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue"), fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "gcp.cue"), strings.Split(e2eRules, "typo_rule")[0])
	input := filepath.Join(t.TempDir(), "records.json")
	modelPath := filepath.Join(pudlDir, "schema", "models", "inventory.cue")
	writeModel := func(complete bool, maxAge string) {
		writeFile(t, modelPath, fmt.Sprintf(`package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Inventory: sm.#SystemModel & {
 name:"inventory"
 observation:{scope:"gcp/prod/firewalls",complete:%t}
 populate:{schema:"pudl/gcp.#Firewall",runs:[{argv:["cat",%q],set:project:"prod"}]}
 checks:[{name:"closed",query:"open_ingress",expect:"empty",severity:"fail",message:"open firewall",evidence:["current"]%s}]
}`, complete, input, maxAge))
		reloadSchemas()
	}
	writeModel(true, "")
	var snapshots []string
	for _, tc := range []struct{ payload, outcome string }{
		{`[{"kind":"compute#firewall","name":"a","direction":"INGRESS","sourceRanges":["0.0.0.0/0"]}]`, "fail"},
		{`[{"kind":"compute#firewall","name":"b","direction":"INGRESS","sourceRanges":["10.0.0.0/8"]}]`, "pass"},
		{`[]`, "pass"},
		{`[{"kind":"compute#firewall","name":"a","direction":"INGRESS","sourceRanges":["0.0.0.0/0"]}]`, "fail"},
	} {
		writeFile(t, input, tc.payload)
		r := runCLI(t, "run", "inventory", "--json", "--detailed-exitcode")
		var report RunReport
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
		require.Len(t, report.Checks, 1)
		require.Equal(t, tc.outcome, report.Checks[0].Outcome, r.Stdout+r.Stderr)
		require.Equal(t, "snapshots", report.Checks[0].Scope)
		require.Len(t, report.Checks[0].Evidence, 1)
		require.True(t, report.Checks[0].Evidence[0].Complete)
		snapshots = append(snapshots, report.Populate.SnapshotID)
	}
	// A partial empty observation cannot establish absence or erase history.
	writeModel(false, "")
	writeFile(t, input, `[]`)
	r := runCLI(t, "run", "inventory", "--json", "--detailed-exitcode")
	require.Equal(t, 1, exitCodeFor(r.Err))
	require.Contains(t, r.Stdout, "incomplete_observation")
	for _, id := range snapshots {
		r := runCLI(t, "snapshot", "show", id, "--json")
		require.NoError(t, r.Err, r.Stderr)
	}
	// Collection failure cannot publish a replacement observation.
	writeFile(t, input, `invalid`)
	r = runCLI(t, "run", "inventory", "--json")
	require.Error(t, r.Err)
	// Freshness is checked on the selected snapshot, not the new check's run.
	writeModel(true, `,max_age:"1ns"`)
	writeFile(t, input, `[]`)
	r = runCLI(t, "run", "inventory", "--json", "--detailed-exitcode")
	require.Equal(t, 1, exitCodeFor(r.Err))
	require.Contains(t, r.Stdout, "stale_observation")
}
