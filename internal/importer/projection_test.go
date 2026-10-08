package importer

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/chazu/pudl/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const projectedFirewallSchema = `package gcp

#Firewall: {
	_pudl: {
		schema_type:   "base"
		resource_type: "gcp.firewall"
		identity_fields: ["project", "name"]
		facts: {
			gcp_firewall: args: {
				project: "project", name: "name"
				disabled: {path: "disabled", default: false}
				network: {path: "network", trim_prefix: "https://www.googleapis.com/compute/v1/"}
			}
			gcp_firewall_source: args: {name: "name", range: "sourceRanges[*]"}
			gcp_firewall_allow: {
				each: "allowed[*]"
				args: {proto: "IPProtocol", port: {path: "ports[*]", default: "*"}}
			}
		}
	}
	kind: "compute#firewall"
	...
}
`

func factArgs(t *testing.T, imp *EnhancedImporter, relation string) []map[string]any {
	t.Helper()
	facts, err := imp.catalogDB.QueryFacts(database.FactFilter{Relation: relation})
	require.NoError(t, err)
	var out []map[string]any
	for _, f := range facts {
		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(f.Args), &args))
		delete(args, "entry_id")
		delete(args, "resource_id")
		out = append(out, args)
	}
	sort.Slice(out, func(i, j int) bool { return toJSON(out[i]) < toJSON(out[j]) })
	return out
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestImportProjectsFactsAndFollowsLatestObservation(t *testing.T) {
	open := `[{"kind":"compute#firewall","project":"p","name":"fw","network":"https://www.googleapis.com/compute/v1/projects/p/global/networks/default","sourceRanges":["0.0.0.0/0"],"allowed":[{"IPProtocol":"tcp","ports":["22","80"]},{"IPProtocol":"icmp"}]}]`
	closed := `[{"kind":"compute#firewall","project":"p","name":"fw","network":"projects/p/global/networks/default","sourceRanges":["10.0.0.0/8"],"allowed":[{"IPProtocol":"tcp","ports":["22"]}],"disabled":true}]`
	imp, openPath := gcpFixture(t, projectedFirewallSchema, "open.json", open)
	closedPath := writeSibling(t, openPath, "closed.json", closed)

	first, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: openPath, ManualSchema: "pudl/gcp.#Firewall"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"gcp_firewall": 1, "gcp_firewall_source": 1, "gcp_firewall_allow": 3}, first.Facts)
	assert.Equal(t, []map[string]any{{"project": "p", "name": "fw", "disabled": false, "network": "projects/p/global/networks/default"}}, factArgs(t, imp, "gcp_firewall"))
	assert.Equal(t, []map[string]any{
		{"port": "*", "proto": "icmp"}, {"port": "22", "proto": "tcp"}, {"port": "80", "proto": "tcp"},
	}, factArgs(t, imp, "gcp_firewall_allow"), "each: keeps proto and port paired per row")

	_, err = imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: closedPath, ManualSchema: "pudl/gcp.#Firewall"})
	require.NoError(t, err)
	assert.Equal(t, []map[string]any{{"name": "fw", "range": "10.0.0.0/8"}}, factArgs(t, imp, "gcp_firewall_source"))

	// The firewall reopens: identical to the first observation, so the whole
	// file deduplicates. Its facts must still become current again.
	again, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: openPath, ManualSchema: "pudl/gcp.#Firewall"})
	require.NoError(t, err)
	require.True(t, again.Skipped)
	assert.Equal(t, []map[string]any{{"name": "fw", "range": "0.0.0.0/0"}}, factArgs(t, imp, "gcp_firewall_source"))
}

func TestImportWarnsAboutZeroYieldRelations(t *testing.T) {
	imp, path := gcpFixture(t, projectedFirewallSchema, "fw.json", `[{"kind":"compute#firewall","project":"p","name":"fw"}]`)
	result, err := imp.ImportFileWithFriendlyIDs(ImportOptions{SourcePath: path, ManualSchema: "pudl/gcp.#Firewall"})
	require.NoError(t, err)
	assert.Contains(t, result.FactWarnings, "relation gcp_firewall_source produced no facts; check its paths against the data")
}
