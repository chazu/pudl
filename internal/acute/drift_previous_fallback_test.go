package acute

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPreviousResourceTypeFallbackMatchesRetainedIdentity(t *testing.T) {
	desired := []map[string]any{{"_schema": "git.repository", "name": "local/demo", "default_branch": "main"}}
	observed := []ObservedRecord{{Data: map[string]any{"_schema": "git.repository", "name": "local/demo", "default_branch": "release"}}}
	history := PreviousInventory{Status: "available", Records: []ObservedRecord{{IdentityFields: []string{"name"}, Data: desired[0]}}}
	drift := InventorySetDiffWithPrevious(desired, observed, func(string) []string { return nil }, history)
	require.Len(t, drift, 1)
	require.Equal(t, "available", drift[0].Fields[0].Previous.Status)
	require.Equal(t, `"main"`, string(drift[0].Fields[0].Previous.Value))
}
