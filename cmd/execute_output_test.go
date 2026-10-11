package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONEarlyFailuresAreSingleDocuments(t *testing.T) {
	cliWorkspace(t)
	for _, args := range [][]string{{"run", "missing-model", "--json"}, {"run", "--json"}, {"query", "bad", "missing_equals", "--json"}, {"not-a-command", "--json"}} {
		r := runCLI(t, args...)
		require.Error(t, r.Err)
		var document map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &document), r.Stdout+r.Stderr)
		require.Equal(t, false, document["ok"])
		require.NotNil(t, document["error"])
	}
}
