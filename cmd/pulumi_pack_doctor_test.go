package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Pulumi pack's graph_detail rules have the same head keys as the
// other packs' graph_detail rules, so with all four packs installed
// `pudl doctor` reports no rule error.
func TestPulumiPackRulesPassDoctorWithOtherPacks(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "pulumi", "gcp", "k8s", "argocd")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "doctor", "--json")
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	var rules *healthDiagnostic
	for i, h := range report.Health {
		if h.Name == "Datalog Rules" {
			rules = &report.Health[i]
		}
	}
	require.NotNil(t, rules, r.Stdout)
	require.Equal(t, "ok", rules.Status, "%s\n%s", rules.Message, rules.Details)
}
