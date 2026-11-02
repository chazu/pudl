package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgressDoesNotContaminateResultJSON(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "check", "missing", "--json", "--progress-json")
	require.Error(t, r.Err)
	require.True(t, json.Valid([]byte(r.Stdout)), r.Stdout)
	require.Contains(t, r.Stderr, `"type":"progress"`)
}
