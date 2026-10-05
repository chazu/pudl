package acute

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPreviousValuesUseLiteralPathsAndExactNull(t *testing.T) {
	desired := []map[string]any{{"name": "x", "a.b": map[string]any{"[0]": []any{json.Number("9007199254740993"), nil}}}}
	current := []ObservedRecord{{Data: map[string]any{"name": "x", "a.b": map[string]any{"[0]": []any{1, true}}}}}
	history := PreviousInventory{Status: "available", SnapshotID: "previous", Records: []ObservedRecord{{Data: desired[0]}}}
	drift := InventorySetDiffWithPrevious(desired, current, nil, history)
	require.Len(t, drift, 1)
	require.Len(t, drift[0].Fields, 2)
	require.Equal(t, "9007199254740993", string(drift[0].Fields[0].Previous.Value))
	require.Equal(t, "null", string(drift[0].Fields[1].Previous.Value))
	require.Equal(t, "available", drift[0].Fields[1].Previous.Status)
	require.Equal(t, "a.b", *drift[0].Fields[0].PathComponents[0].Key)
	require.Equal(t, "[0]", *drift[0].Fields[0].PathComponents[1].Key)
}
func TestPreviousValuesStatusesAndIdentityContract(t *testing.T) {
	desired := []map[string]any{{"_schema": "S", "name": "x", "state": "wanted"}}
	observed := []ObservedRecord{{Data: map[string]any{"_schema": "S", "name": "x", "state": "now"}}}
	for _, status := range []string{"no-baseline", "pruned"} {
		drift := InventorySetDiffWithPrevious(desired, observed, nil, PreviousInventory{Status: status})
		require.Equal(t, status, drift[0].Fields[0].Previous.Status)
	}
	history := PreviousInventory{Status: "available", Records: []ObservedRecord{}}
	drift := InventorySetDiffWithPrevious(desired, observed, nil, history)
	require.Equal(t, "absent", drift[0].Fields[0].Previous.Status)
	history.Records = []ObservedRecord{{Data: map[string]any{"_schema": "S", "name": "x", "state": "a"}}, {Data: map[string]any{"_schema": "S", "name": "x", "state": "b"}}}
	drift = InventorySetDiffWithPrevious(desired, observed, nil, history)
	require.Equal(t, "ambiguous", drift[0].Fields[0].Previous.Status)
	drift = InventorySetDiffWithPrevious(desired, observed, func(string) []string { return []string{"name"} }, history)
	require.Equal(t, "incompatible", drift[0].Fields[0].Previous.Status)
	history.Records = []ObservedRecord{{IdentityFields: []string{"name"}, Data: map[string]any{"_schema": "S", "name": "x", "state": "old"}}}
	drift = InventorySetDiffWithPrevious(desired, observed, func(string) []string { return []string{"name"} }, history)
	require.Equal(t, `"old"`, string(drift[0].Fields[0].Previous.Value))
}
