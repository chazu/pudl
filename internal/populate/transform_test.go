package populate

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

func transform(t *testing.T, src string) *systemmodel.Transform {
	t.Helper()
	tr, err := systemmodel.NewTransform(cuecontext.New().CompileString(src))
	require.NoError(t, err)
	return tr
}

// The transform runs before set: it unwraps, drops on null, and set still
// assigns its fields on the records kept.
func TestRunAppliesTransformBeforeSet(t *testing.T) {
	run := shellRun(`echo '[{"t":"keep","r":{"name":"a","n":12345678901234567890}},{"t":"drop","r":{"name":"b"}},{"t":"keep","r":{"name":"c","project":"wrong"}}]'`,
		map[string]any{"project": "p1"})
	run.Transform = transform(t, `{in: _, out: [if in.t == "drop" {null}, in.r][0]}`)
	res, err := Run(context.Background(), systemmodel.Populate{Runs: []systemmodel.CommandRun{run}}, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.Empty(t, res.Failed)
	encoded, err := json.Marshal(res.Records)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"a","n":12345678901234567890,"project":"p1"},{"name":"c","project":"p1"}]`, string(encoded))
}

// A record the transform cannot handle fails the run, as any malformed
// output does; the other runs are kept.
func TestRunTransformErrorIsAFailedInput(t *testing.T) {
	bad := shellRun(`echo '{"name":"a"}'`, nil)
	bad.Transform = transform(t, `{in: _, out: in.missing}`)
	p := systemmodel.Populate{Runs: []systemmodel.CommandRun{bad, shellRun(`echo '{"name":"b"}'`, nil)}}
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
	assert.Contains(t, res.Failed[0].Error, "stdout record 0: transform")
	require.Len(t, res.Records, 1)
}

// Enumerated runs keep their transform.
func TestRunTransformSurvivesEach(t *testing.T) {
	run := shellRun(`echo '{"r":{"name":"{{item}}"}}'`, nil)
	run.Argv = []string{"sh", "-c", `echo '{"r":{"name":"x"}}'`, "{{item}}"}
	run.Transform = transform(t, `{in: _, out: in.r}`)
	p := systemmodel.Populate{Each: &systemmodel.Enumeration{Argv: []string{"echo", `["a","b"]`}}, Runs: []systemmodel.CommandRun{run}}
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	encoded, _ := json.Marshal(res.Records)
	assert.JSONEq(t, `[{"name":"x"},{"name":"x"}]`, string(encoded))
}

// A kubectl get -o json prints one List object; `out: in.items` stores one
// record per item, and set applies to each of them.
func TestRunTransformUnwrapsListItems(t *testing.T) {
	// Runs execute in the model directory, so the fixture's path is absolute.
	fixture, err := filepath.Abs(filepath.Join("testdata", "kubectl_list.json"))
	require.NoError(t, err)
	run := shellRun(`cat "$0"`, map[string]any{"cluster": "198.51.100.30"})
	run.Argv = append(run.Argv, fixture)
	run.Transform = transform(t, `{in: _, out: in.items}`)
	res, err := Run(context.Background(), systemmodel.Populate{Runs: []systemmodel.CommandRun{run}}, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.Empty(t, res.Failed)
	encoded, err := json.Marshal(res.Records)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"payments","uid":"5c1d2d3e-0000-4000-8000-000000000001"},"cluster":"198.51.100.30"},
		{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"catalog","uid":"5c1d2d3e-0000-4000-8000-000000000002"},"cluster":"198.51.100.30"}
	]`, string(encoded))
}

// An item that is not an object fails the run, as any malformed output does.
func TestRunTransformListElementMustBeAnObject(t *testing.T) {
	run := shellRun(`echo '{"kind":"List","items":[{"name":"a"},"b"]}'`, nil)
	run.Transform = transform(t, `{in: _, out: in.items}`)
	res, err := Run(context.Background(), systemmodel.Populate{Runs: []systemmodel.CommandRun{run}}, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.Empty(t, res.Records)
	require.Len(t, res.Failed, 1)
	assert.Contains(t, res.Failed[0].Error, "out[1] is string, not an object")
}
