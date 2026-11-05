package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphvocab"
)

func TestGraphVocabMatchesExportAndReportsProblems(t *testing.T) {
	ws := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	vocab := func() graphvocab.Vocabulary {
		t.Helper()
		r := runCLI(t, "graph", "vocab", "--json")
		require.NoError(t, r.Err, r.Stdout+r.Stderr)
		var v graphvocab.Vocabulary
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), r.Stdout)
		return v
	}
	clean := vocab()
	require.Empty(t, clean.Problems)
	g := exportGraph(t)
	require.Equal(t, len(g.Vocabulary.ResourceTypes), len(clean.ResourceTypes), "--json is the export's vocabulary")
	require.Equal(t, len(g.Vocabulary.EdgeTypes), len(clean.EdgeTypes))

	text := runCLI(t, "graph", "vocab")
	require.NoError(t, text.Err)
	require.Contains(t, text.Stdout, "gcp.compute.subnetwork")
	require.Contains(t, text.Stdout, "network(group, via in_network, region)")
	require.Contains(t, text.Stdout, "No problems.")

	// A rule deriving an edge type nobody declared, and a role naming one.
	rule := `package rules

acme_graph_mystery: {
	head: {rel: "graph_edge", args: {type: "mystery_link", source: "$R", target: "$N", basis: "observed"}}
	body: [{rel: "gcp_subnet", args: {resource_id: "$R", network: "$N"}}]
}
`
	require.NoError(t, os.WriteFile(filepath.Join(ws, "schema", "pudl", "rules", "acme_graph.cue"), []byte(rule), 0o644))
	broken := vocab()
	require.Len(t, broken.Problems, 1)
	require.Contains(t, broken.Problems[0], `derives graph_edge type "mystery_link"`)

	doc := runCLI(t, "doctor", "--health-only")
	require.Contains(t, doc.Stdout, "Graph Vocabulary")
	require.Contains(t, doc.Stdout, "mystery_link")
}
