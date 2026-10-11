package acute

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInventoryIdentityUsesSharedNestedPaths(t *testing.T) {
	resolve := func(string) []string {
		return []string{"metadata.namespace", `metadata.labels."cloud.example/location"`}
	}
	record := func(namespace, location string) map[string]any {
		return map[string]any{"_schema": "thing", "name": "fallback", "metadata": map[string]any{"namespace": namespace, "labels": map[string]any{"cloud.example/location": location}}}
	}
	a, _, ok := RecordIdentity(record("a/b", "c"), resolve)
	require.True(t, ok)
	b, _, ok := RecordIdentity(record("a", "b/c"), resolve)
	require.True(t, ok)
	require.NotEqual(t, a, b, "composite identity values cannot collide through delimiters")
	_, _, ok = RecordIdentity(map[string]any{"_schema": "thing", "name": "fallback"}, resolve)
	require.False(t, ok, "a missing declared identity must not silently match by an unrelated name")
	diffs := InventorySetDiff([]map[string]any{record("a", "b")}, []ObservedRecord{{Data: record("a", "b")}}, resolve)
	require.Empty(t, diffs)
}
