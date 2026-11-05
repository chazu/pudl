package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pudl-qaw: upgrading a pack whose types changed their _pudl.facts specs left
// stored records with the facts the old specs projected, until pudl doctor
// --fix --only projections ran. The install now reprojects them.
func TestPackUpgradeReprojectsStoredRecords(t *testing.T) {
	ws := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	// An older pack version projected firewalls to another relation.
	network := filepath.Join(ws, "schema", "gcp", "network.cue")
	data, err := os.ReadFile(network)
	require.NoError(t, err)
	current := string(data)
	require.Contains(t, current, "gcp_firewall_network: args:")
	writeFile(t, network, strings.Replace(current, "gcp_firewall_network: args:", "gcp_firewall_network_v0: args:", 1))
	reloadSchemas()

	r = runCLI(t, "check", "gcp-fixture-firewalls")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "query", "gcp_firewall_network_v0", "--json")
	require.NoError(t, r.Err, r.Stderr)
	require.Contains(t, r.Stdout, "allow-ssh-world", "the old spec's facts are stored")

	r = runCLI(t, "pack", "install", "gcp", "--overwrite")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stderr, "reprojected")

	r = runCLI(t, "query", "gcp_firewall_network", "--json")
	require.NoError(t, r.Err, r.Stderr)
	require.Contains(t, r.Stdout, "allow-ssh-world", "records carry the new spec's facts without a doctor run")
	require.NotContains(t, r.Stderr, "doctor --fix --only projections")
}

// Installing into a workspace with no catalog does not create one.
func TestPackInstallDoesNotCreateACatalog(t *testing.T) {
	ws := cliWorkspace(t)
	catalog := filepath.Join(ws, "data", "sqlite", "catalog.db")
	if _, err := os.Stat(catalog); err == nil {
		t.Skip("pudl init created the catalog")
	}
	r := runCLI(t, "pack", "install", "aws")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NoFileExists(t, catalog)
}
