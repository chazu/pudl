package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/ui"
	"github.com/stretchr/testify/require"
)

func TestGCPNetworkHygieneWorkflow(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "example", "install", "gcp-network-hygiene", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "example", "install", "gcp-network-hygiene", "--json")
	require.NoError(t, r.Err, "identical install is idempotent")
	r = runCLI(t, "run", "gcp-network-hygiene", "--json", "--detailed-exitcode")
	require.Equal(t, 2, exitCodeFor(r.Err), r.Stdout+r.Stderr)
	var before RunReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &before))
	require.Equal(t, 1, before.Checks[0].Count, "DENY and private-source rules are not internet ALLOW findings")
	require.Len(t, before.Checks[0].Witnesses, 1)
	require.NotEmpty(t, before.Checks[0].Witnesses[0].EvidenceIDs)
	r = runCLI(t, "list", "--collection-id", before.Populate.SnapshotID, "--where", "name=public-ssh", "--select", "name", "--select", "sourceRanges[*]", "--all", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var listing ui.ListOutput
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &listing))
	require.Len(t, listing.Entries, 1)
	entryID := listing.Entries[0].ID
	require.Equal(t, "public-ssh", listing.Entries[0].Fields["name"])
	r = runCLI(t, "show", entryID, "--field", "sourceRanges[*]")
	require.NoError(t, r.Err)
	require.JSONEq(t, `["0.0.0.0/0"]`, r.Stdout)
	r = runCLI(t, "model", "new", "saved-firewall-check", "--check", "gcp_internet_ingress_allow", "--evidence", "scope:fixture/example-project/firewalls", "--max-age", "15m", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	reloadSchemas()
	r = runCLI(t, "run", "saved-firewall-check", "--json", "--detailed-exitcode")
	require.Equal(t, 2, exitCodeFor(r.Err))
	fixed, err := os.ReadFile(filepath.Join(pudlDir, "populators", "gcp-network-hygiene", "fixed.json"))
	require.NoError(t, err)
	writeFile(t, filepath.Join(pudlDir, "populators", "gcp-network-hygiene", "current.json"), string(fixed))
	r = runCLI(t, "run", "gcp-network-hygiene", "--json", "--detailed-exitcode")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var after RunReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &after))
	require.Equal(t, "pass", after.Checks[0].Outcome)
	r = runCLI(t, "run", "saved-firewall-check", "--json", "--detailed-exitcode")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "snapshot", "show", after.Populate.SnapshotID, "--compare", before.Populate.SnapshotID, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var compared struct {
		Changes []snapshotChange `json:"changes"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &compared))
	require.Len(t, compared.Changes, 1)
	require.Equal(t, "changed", compared.Changes[0].Change)
	r = runCLI(t, "show", entryID, "--history", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, before.Populate.SnapshotID)
	require.Contains(t, r.Stdout, after.Populate.SnapshotID)
	r = runCLI(t, "run", "report", before.RunID, "--json")
	require.NoError(t, r.Err)
	require.Contains(t, r.Stdout, "0.0.0.0/0", "original evidence remains inspectable")
}

func TestPayloadFilteringPrecedesPaginationAndPreservesFancy(t *testing.T) {
	cliWorkspace(t)
	file := filepath.Join(t.TempDir(), "records.json")
	var records []map[string]any
	for i := 0; i < 45; i++ {
		records = append(records, map[string]any{"id": i, "enabled": i >= 25, "optional": nil})
	}
	data, err := json.Marshal(records)
	require.NoError(t, err)
	writeFile(t, file, string(data))
	r := runCLI(t, "import", file, "--json")
	require.NoError(t, r.Err)
	r = runCLI(t, "list", "--items-only", "--where", "enabled=true", "--select", "id", "--select", "optional", "--select", "missing", "--all", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var result ui.ListOutput
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &result))
	require.Equal(t, 20, result.TotalMatched)
	require.Len(t, result.Entries, 20)
	require.Contains(t, result.Entries[0].Fields, "optional")
	require.Contains(t, result.Entries[0].MissingFields, "missing")
	r = runCLI(t, "list", "--items-only", "--all", "--json")
	require.NoError(t, r.Err)
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &result))
	require.Len(t, result.Entries, 45)
	r = runCLI(t, "help", "list", "--json")
	require.NoError(t, r.Err)
	require.Contains(t, r.Stdout, `"name": "fancy"`)
}
