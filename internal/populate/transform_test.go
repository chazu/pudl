package populate

import (
	"context"
	"encoding/json"
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
