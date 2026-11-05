package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphvocab"
)

// A workspace whose built-ins came from an older pudl (no stamp, no
// graph_vocabulary.cue) used to report every built-in edge type a pack's
// rules derive as undeclared, until something ran an import.
func TestReadCommandRefreshesStaleBuiltins(t *testing.T) {
	ws := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	schemaPudl := filepath.Join(ws, "schema", "pudl")
	require.NoError(t, os.Remove(filepath.Join(schemaPudl, "rules", "graph_vocabulary.cue")))
	require.NoError(t, os.Remove(filepath.Join(schemaPudl, ".bootstrap-digest")))

	r = runCLI(t, "graph", "vocab", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stderr, "Updated pudl's built-in schemas")
	var v graphvocab.Vocabulary
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), r.Stdout)
	require.Empty(t, v.Problems)
	require.NotEmpty(t, v.EdgeTypes)

	again := runCLI(t, "graph", "vocab", "--json")
	require.NoError(t, again.Err)
	require.NotContains(t, again.Stderr, "Updated pudl's built-in schemas", "refreshes once per pudl version")
}
