package ownership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRejectsInvalidOwnerBlocks(t *testing.T) {
	for name, tc := range map[string]struct{ raw, err string }{
		"unknown key":        {`{"paths": ["labels.team"], "team": "x"}`, `unknown field "team"`},
		"empty block":        {`{}`, "declare at least one of paths, map or fallback"},
		"wildcard path":      {`{"paths": ["tags[*].Value"]}`, "cannot contain [*]"},
		"bad path":           {`{"paths": [""]}`, "empty field path"},
		"paths not a list":   {`{"paths": "labels.team"}`, "must be {paths?"},
		"map without path":   {`{"map": {"owners": {"p": "t"}}}`, "map.path"},
		"map without table":  {`{"map": {"path": "project"}}`, "map.owners is required"},
		"empty mapped owner": {`{"map": {"path": "project", "owners": {"p": ""}}}`, `map.owners."p" is empty`},
		"empty fallback":     {`{"fallback": ""}`, "fallback is empty"},
		"non-string owner":   {`{"fallback": 3}`, "must be {paths?"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.err)
		})
	}
}

func TestResolveTriesPathsThenMapThenFallback(t *testing.T) {
	spec, err := Parse([]byte(`{
		"paths": ["labels.team", "tags.Owner"],
		"map": {"path": "project", "owners": {"proj-a": "payments"}},
		"fallback": "platform"
	}`))
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		record     map[string]any
		owner, via string
	}{
		"first path":          {map[string]any{"labels": map[string]any{"team": "orders"}, "tags": map[string]any{"Owner": "x"}}, "orders", "path:labels.team"},
		"second path":         {map[string]any{"tags": map[string]any{"Owner": "search"}, "project": "proj-a"}, "search", "path:tags.Owner"},
		"empty label skipped": {map[string]any{"labels": map[string]any{"team": ""}, "project": "proj-a"}, "payments", "map:project"},
		"non-string skipped":  {map[string]any{"labels": map[string]any{"team": 3}, "project": "proj-a"}, "payments", "map:project"},
		"unmapped project":    {map[string]any{"project": "proj-z"}, "platform", "fallback"},
	} {
		t.Run(name, func(t *testing.T) {
			owner, via, ok := spec.Resolve(tc.record)
			require.True(t, ok)
			assert.Equal(t, tc.owner, owner)
			assert.Equal(t, tc.via, via)
		})
	}

	noFallback, err := Parse([]byte(`{"paths": ["labels.team"]}`))
	require.NoError(t, err)
	_, _, ok := noFallback.Resolve(map[string]any{"name": "x"})
	assert.False(t, ok, "nothing names an owner: unowned")
}

func TestCanonicalIsStable(t *testing.T) {
	a, err := Parse([]byte(`{"map": {"path": "project", "owners": {"b": "y", "a": "x"}}}`))
	require.NoError(t, err)
	b, err := Parse([]byte(`{"map": {"path": "project", "owners": {"a": "x", "b": "y"}}}`))
	require.NoError(t, err)
	assert.Equal(t, a.Canonical(), b.Canonical())
	c, err := Parse([]byte(`{"map": {"path": "project", "owners": {"a": "x", "b": "z"}}}`))
	require.NoError(t, err)
	assert.NotEqual(t, a.Canonical(), c.Canonical(), "a mapping change changes the fingerprint")
	assert.Equal(t, "", (*Spec)(nil).Canonical())
}
