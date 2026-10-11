package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckMissingEvidenceNeverPasses(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "rules", "missing.cue"), `package rules
missing_check: {head: {rel:"missing_check",args:{name:"$N"}}, body:[{rel:"missing_evidence",args:{name:"$N"}}]}
`)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "missing.cue"), `package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Missing: sm.#SystemModel & {name:"missing", checks:[{name:"check",query:"missing_check",expect:"empty",severity:"fail",message:"must have evidence"}]}
`)
	r := runCLI(t, "run", "missing", "--json", "--detailed-exitcode")
	require.Error(t, r.Err)
	require.Equal(t, 1, exitCodeFor(r.Err))
	var report RunReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	require.False(t, report.OK)
	require.Len(t, report.Checks, 1)
	require.Equal(t, "unknown", report.Checks[0].Outcome)
	require.False(t, report.Checks[0].Passed)
	require.NotEmpty(t, report.Checks[0].Diagnostics)
	stored := runCLI(t, "run", "report", report.RunID, "--json")
	require.NoError(t, stored.Err)
	require.Contains(t, stored.Stdout, "missing_evidence")
	require.NoError(t, json.Unmarshal([]byte(stored.Stdout), &report))
	require.Equal(t, "unknown", report.Checks[0].Outcome)
}

func TestCommandEmptyInventoryAndBrokenProjectionHealth(t *testing.T) {
	pudlDir := cliWorkspace(t)
	schemaPath := filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue")
	writeFile(t, schemaPath, fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "empty.cue"), `package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Empty: sm.#SystemModel & {
 name:"empty"
 populate: {schema:"pudl/gcp.#Firewall",runs:[{argv:["printf","[]"]}]}
 checks:[{name:"empty",query:"gcp_firewall",expect:"empty",severity:"fail",message:"empty inventory"}]
}
`)
	r := runCLI(t, "run", "empty", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var report RunReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report))
	require.NotNil(t, report.Populate)
	require.Zero(t, report.Populate.Records)
	require.NotEmpty(t, report.Populate.SnapshotID)
	require.Equal(t, "pass", report.Checks[0].Outcome)
	// Existing relation names remain attributable when their specs break.
	writeFile(t, schemaPath, fmt.Sprintf(e2eFirewallBase, `facts: gcp_firewall: args: {a:"a[*]",b:"b[*]"}`))
	reloadSchemas()
	r = runCLI(t, "run", "empty", "--json", "--detailed-exitcode")
	require.Error(t, r.Err)
	require.Equal(t, 1, exitCodeFor(r.Err))
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	require.Equal(t, "unknown", report.Checks[0].Outcome)
	require.Contains(t, r.Stdout, "invalid_projection")
}
