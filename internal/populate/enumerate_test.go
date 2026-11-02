package populate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

// writeFixtures writes one file per name with the given content and returns
// the directory.
func writeFixtures(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	return dir
}

// each's command lists objects; select picks each input, and every run is
// repeated per input with the placeholder substituted in argv and set.
func TestRunExpandsEnumeratedInputs(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"projects.json": `[{"projectId":"p1"},{"projectId":"p2"},{"projectId":"p1"}]`,
		"p1.json":       `[{"name":"fw-a"}]`,
		"p2.json":       `[{"name":"fw-b"},{"name":"fw-c"}]`,
	})
	p := systemmodel.Populate{
		Each: &systemmodel.Enumeration{Argv: []string{"cat", "projects.json"}, Select: "projectId"},
		Runs: []systemmodel.CommandRun{{Argv: []string{"cat", "{{item}}.json"}, Set: map[string]any{"project": "{{item}}"}}},
	}
	res, err := Run(context.Background(), p, Request{ModelDir: dir})
	require.NoError(t, err)
	assert.Equal(t, []string{"p1", "p2"}, res.Inputs, "inputs are recorded in order, without duplicates")
	assert.Equal(t, 2, res.Runs)
	assert.Empty(t, res.Failed)
	encoded, err := json.Marshal(res.Records)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"name":"fw-a","project":"p1"},
		{"name":"fw-b","project":"p2"},
		{"name":"fw-c","project":"p2"}
	]`, string(encoded))
}

// A failed run of an enumerated input names that input.
func TestRunNamesTheFailedEnumeratedInput(t *testing.T) {
	dir := writeFixtures(t, map[string]string{
		"projects.json": `["p1", "p2"]`,
		"p1.json":       `[{"name":"fw-a"}]`,
	})
	p := systemmodel.Populate{
		Each: &systemmodel.Enumeration{Argv: []string{"cat", "projects.json"}},
		Runs: []systemmodel.CommandRun{{Argv: []string{"cat", "{{item}}.json"}}},
	}
	res, err := Run(context.Background(), p, Request{ModelDir: dir})
	require.NoError(t, err)
	require.Len(t, res.Records, 1)
	require.Len(t, res.Failed, 1)
	assert.Equal(t, "p2", res.Failed[0].Item)
	assert.Equal(t, "p2", res.Failed[0].Label())
	assert.Equal(t, []string{"cat", "p2.json"}, res.Failed[0].Argv)
	assert.Equal(t, 1, res.Failed[0].ExitStatus)
	assert.Contains(t, res.Failed[0].Stderr, "p2.json")
}

// Without its inputs there is no fan-out, so a failed or malformed
// enumeration fails the populate.
func TestRunFailsWhenTheEnumerationFails(t *testing.T) {
	dir := writeFixtures(t, map[string]string{"objects.json": `[{"projectId":"p1"}]`, "numbers.json": `[{"projectId":{}}]`})
	runs := []systemmodel.CommandRun{{Argv: []string{"cat", "{{item}}.json"}}}
	cases := map[string]struct {
		each systemmodel.Enumeration
		want string
	}{
		"command fails":       {systemmodel.Enumeration{Argv: []string{"cat", "missing.json"}}, "populate each (cat missing.json): exit status 1"},
		"objects, no select":  {systemmodel.Enumeration{Argv: []string{"cat", "objects.json"}}, "item 0 is map[string]interface {}, not a string (set select to pick a field)"},
		"select absent":       {systemmodel.Enumeration{Argv: []string{"cat", "objects.json"}, Select: "id"}, "item 0 has no id"},
		"select not a string": {systemmodel.Enumeration{Argv: []string{"cat", "numbers.json"}, Select: "projectId"}, "item 0 projectId is map[string]interface {}, not a string"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			each := tc.each
			_, err := Run(context.Background(), systemmodel.Populate{Each: &each, Runs: runs}, Request{ModelDir: dir})
			require.ErrorContains(t, err, tc.want)
		})
	}
}
