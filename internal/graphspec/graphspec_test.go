package graphspec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAndEval(t *testing.T) {
	spec, err := Parse(json.RawMessage(`{
		"name": "name",
		"account": "project",
		"region": {"path": "region", "last_segment": true},
		"refs": [{"path": "selfLink", "trim_prefix": "https://x/v1/"}, "id"]
	}`))
	require.NoError(t, err)
	rec := map[string]any{
		"name":     "net-a",
		"project":  "p1",
		"region":   "https://x/v1/projects/p1/regions/us-central1",
		"selfLink": "https://x/v1/projects/p1/global/networks/net-a",
		"id":       json.Number("42"),
	}
	require.Equal(t, "net-a", Eval(spec.Name, rec))
	require.Equal(t, "p1", Eval(spec.Account, rec))
	require.Equal(t, "us-central1", Eval(spec.Region, rec))
	require.Equal(t, "", Eval(spec.Zone, rec))
	require.Equal(t, []string{"projects/p1/global/networks/net-a", "42"}, spec.RefValues(rec))
}

func TestParseRejectsMistakes(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown key":       `{"nmae": "name"}`,
		"unknown value key": `{"name": {"path": "name", "trim": "x"}}`,
		"wildcard":          `{"refs": ["items[*].id"]}`,
		"empty path":        `{"name": {"path": ""}}`,
		"not a path":        `{"name": 3}`,
	} {
		_, err := Parse(json.RawMessage(raw))
		require.Error(t, err, name)
	}
}

func TestEvalIgnoresNonScalars(t *testing.T) {
	spec, err := Parse(json.RawMessage(`{"name": "tags"}`))
	require.NoError(t, err)
	require.Equal(t, "", Eval(spec.Name, map[string]any{"tags": map[string]any{"a": "b"}}))
	var nilSpec *Spec
	require.Nil(t, nilSpec.RefValues(map[string]any{}))
}
