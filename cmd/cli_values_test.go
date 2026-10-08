package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCLIValueTypesExactly(t *testing.T) {
	assert.Equal(t, json.Number("123456789012345678901"), parseCLIValue("123456789012345678901"))
	assert.Equal(t, "123", parseCLIValue(`"123"`))
	assert.Equal(t, true, parseCLIValue("true"))
	assert.Nil(t, parseCLIValue("null"))
	assert.Equal(t, "prod-a", parseCLIValue("prod-a"))
	assert.Equal(t, "00123", parseCLIValue("00123"))
	assert.Equal(t, []any{"a"}, parseCLIValue(`["a"]`))
}

func TestParseCLIAssignments(t *testing.T) {
	got, err := parseCLIAssignments("set", []string{"project=prod-a", `metadata."a.b"=1`})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "project", got[0].Path.String())
	assert.Equal(t, json.Number("1"), got[1].Value)

	_, err = parseCLIAssignments("set", []string{"list[*]=1"})
	require.Error(t, err)
	_, err = parseCLIAssignments("set", []string{"novalue"})
	require.Error(t, err)
}

func TestQueryConstraintBooleansAreTyped(t *testing.T) {
	constraints, err := parseQueryConstraints([]string{"disabled=false", "name=fw"})
	require.NoError(t, err)
	assert.Equal(t, false, constraints["disabled"])
	assert.Equal(t, "fw", constraints["name"])
}
