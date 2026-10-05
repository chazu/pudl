package acute

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 1 and "1" printed identically under the old fmt.Sprint comparison, so a type
// change was reported as no drift at all.
func TestCompareDesired_NumberIsNotString(t *testing.T) {
	fields := CompareDesired(map[string]any{"port": int64(1)}, map[string]any{"port": "1"})
	require.Equal(t, []FieldDiff{{Path: "port", Expected: int64(1), Observed: "1"}}, fields)
	assert.Equal(t, `port: "1" → want 1`, fields[0].String())
	assert.Equal(t, `port: expected 1, observed "1"`, fields[0].Detail())
}

func TestCompareDesired_NumbersCompareByValue(t *testing.T) {
	desired := map[string]any{"a": int64(3), "b": 2.5, "c": json.Number("9007199254740993"), "d": big.NewInt(7)}
	observed := map[string]any{"a": float64(3), "b": json.Number("2.50"), "c": json.Number("9007199254740993"), "d": 7}
	assert.Empty(t, CompareDesired(desired, observed))

	// Exact: a float64 round trip would call these equal.
	fields := CompareDesired(map[string]any{"c": json.Number("9007199254740993")}, map[string]any{"c": json.Number("9007199254740992")})
	require.Len(t, fields, 1)
	assert.Equal(t, "c", fields[0].Path)
}

func TestCompareDesired_NestedPaths(t *testing.T) {
	desired := map[string]any{
		"_schema": "ignored",
		"spec": map[string]any{
			"replicas": int64(3),
			"template": map[string]any{"image": "app:v2"},
			"ports":    []any{map[string]any{"port": int64(80)}},
		},
	}
	observed := map[string]any{
		"spec": map[string]any{
			"replicas": float64(2),
			"template": map[string]any{"image": "app:v1", "debug": true},
			"ports":    []any{map[string]any{"port": float64(8080)}},
		},
		"status": "extra top-level field is fine",
	}
	assert.Equal(t, []FieldDiff{
		{Path: "spec.ports[0].port", Expected: int64(80), Observed: float64(8080)},
		{Path: "spec.replicas", Expected: int64(3), Observed: float64(2)},
		{Path: "spec.template.debug", Observed: true, Unexpected: true},
		{Path: "spec.template.image", Expected: "app:v2", Observed: "app:v1"},
	}, CompareDesired(desired, observed))
}

func TestCompareDesired_ListLengthMismatchReportsWholeList(t *testing.T) {
	fields := CompareDesired(map[string]any{"tags": []any{"a", "b"}}, map[string]any{"tags": []any{"a"}})
	require.Len(t, fields, 1)
	assert.Equal(t, "tags", fields[0].Path)
}

func TestCompareDesired_MissingNestedField(t *testing.T) {
	fields := CompareDesired(
		map[string]any{"meta": map[string]any{"owner": "ops"}},
		map[string]any{"meta": map[string]any{}},
	)
	assert.Equal(t, []FieldDiff{{Path: "meta.owner", Expected: "ops", Missing: true}}, fields)
}

func TestValuesEqual(t *testing.T) {
	assert.True(t, ValuesEqual(nil, nil))
	assert.False(t, ValuesEqual(nil, ""))
	assert.False(t, ValuesEqual(true, "true"))
	assert.False(t, ValuesEqual(int64(0), false))
	assert.True(t, ValuesEqual([]any{int64(1), "x"}, []any{float64(1), "x"}))
	assert.False(t, ValuesEqual(map[string]any{"a": 1}, map[string]any{"a": 1, "b": 2}))
}
