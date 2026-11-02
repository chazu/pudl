package systemmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalOmitsPopulateForChecksOnlyModel(t *testing.T) {
	checksOnly := SystemModel{Name: "posture", Populate: Populate{Absent: true}, Checks: []Check{{Name: "c", Query: "q", Expect: "empty", Severity: "fail", Message: "m"}}}
	b, err := json.Marshal(checksOnly)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "populate")

	b, err = json.Marshal(SystemModel{Name: "posture"})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "populate", "a zero Populate is not emitted either")

	command := SystemModel{Name: "fw", Populate: Populate{Runs: []CommandRun{{Argv: []string{"true"}}}}}
	b, err = json.Marshal(command)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"populate":{"runs":[{"argv":["true"]}]}`)
}

func TestWithoutLegacyEmptyPopulate(t *testing.T) {
	legacy := map[string]any{"name": "posture", "populate": map[string]any{}}
	got, ok := WithoutLegacyEmptyPopulate(SchemaName, legacy)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"name": "posture"}, got)
	assert.Contains(t, legacy, "populate", "the input is not mutated")

	for name, tc := range map[string]struct {
		schema string
		data   any
	}{
		"other schema":       {"pudl/core.#Item", legacy},
		"populated":          {SchemaName, map[string]any{"name": "fw", "populate": map[string]any{"runs": []any{}}}},
		"no populate":        {SchemaName, map[string]any{"name": "posture"}},
		"non-object payload": {SchemaName, "text"},
	} {
		got, ok := WithoutLegacyEmptyPopulate(tc.schema, tc.data)
		assert.False(t, ok, name)
		assert.Equal(t, tc.data, got, name)
	}
}
