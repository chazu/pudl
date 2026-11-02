package acute

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

// Matching and previous-value lookup share IdentityFields, so a record keyed
// by the second fallback key (path) is matched and compared by path in both.
func TestIdentityFallbackIsSharedByMatchingAndHistory(t *testing.T) {
	desired := map[string]any{"_schema": "fs.file", "path": "/etc/motd", "mode": "0644"}
	fields, fallback := IdentityFields(desired, nil)
	require.True(t, fallback)
	require.Equal(t, []string{"path"}, fields)
	_, label, ok := RecordIdentity(desired, nil)
	require.True(t, ok)
	require.Equal(t, "fs.file//etc/motd", label)

	observed := []ObservedRecord{{Data: map[string]any{"_schema": "fs.file", "path": "/etc/motd", "mode": "0600"}}}
	history := PreviousInventory{Status: "available", Records: []ObservedRecord{{Data: map[string]any{"_schema": "fs.file", "path": "/etc/motd", "mode": "0640"}}}}
	drift := InventorySetDiffWithPrevious([]map[string]any{desired}, observed, nil, history)
	require.Len(t, drift, 1)
	require.Equal(t, `"0640"`, string(drift[0].Fields[0].Previous.Value))
}

func TestIdentityFallbacksReportedOncePerSchema(t *testing.T) {
	identity := func(schema string) []string {
		if schema == "declared" {
			return []string{"uid"}
		}
		return nil
	}
	desired := []map[string]any{
		{"_schema": "bare", "name": "a"},
		{"_schema": "bare", "name": "b"},
		{"_schema": "declared", "uid": "1", "name": "x"},
		{"_schema": "nothing"},
	}
	observed := []ObservedRecord{{Data: map[string]any{"_schema": "bare", "id": "c"}}, {Data: map[string]any{"_schema": "bare", "name": "a"}}}
	fallbacks := IdentityFallbacks(identity, desired, observed)
	require.Equal(t, map[string][]string{"bare": {"name", "id"}}, fallbacks)

	var human strings.Builder
	ModelDriftResult{Clean: true, IdentityFallbacks: fallbacks}.WriteMarkdown(&human)
	require.Contains(t, human.String(), "- identity fallback: bare declares no identity_fields (or is not loaded); records matched by name, id\n")
	require.Empty(t, IdentityFallbacks(identity, desired[2:3], nil))
}
