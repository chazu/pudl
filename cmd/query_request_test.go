package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryRejectsInvalidRequests(t *testing.T) {
	cliWorkspace(t)
	for _, args := range [][]string{
		{"query", "facts", "name"},
		{"query", "facts", "=value"},
		{"query", "facts", "  =value"},
		{"query", "facts", "name=first", "name=second"},
		{"query", "facts", "--max-iterations=-1"},
		{"query", "--list", "facts"},
		{"query", "--list", "--topo"},
	} {
		t.Run(args[len(args)-1], func(t *testing.T) {
			result := runCLI(t, args...)
			require.Error(t, result.Err)
			require.Empty(t, result.Stdout)
		})
	}
}

func TestQueryConstraintsKeepEmptyValuesAndEquals(t *testing.T) {
	constraints, err := parseQueryConstraints([]string{"empty=", "label=a=b", "literal=00123", `quoted="123"`, "number=9007199254740993", "odd.field=value"})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"empty": "", "label": "a=b", "literal": "00123", "quoted": "123",
		"number": int64(9007199254740993), "odd.field": "value",
	}, constraints)
}
