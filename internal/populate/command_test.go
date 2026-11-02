package populate

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/chazu/pudl/internal/systemmodel"
)

func shellRun(script string, set map[string]any) systemmodel.CommandRun {
	return systemmodel.CommandRun{Argv: []string{"sh", "-c", script}, Set: set}
}

// Arrays contribute their elements, objects are one record each, numbers stay
// exact, and nested `set` assigns leaves without replacing their parents.
func TestRunDecodesRecordStreamsAndAppliesSet(t *testing.T) {
	set := map[string]any{"metadata": map[string]any{"labels": map[string]any{"env": "prod"}}}
	p := systemmodel.Populate{Runs: []systemmodel.CommandRun{
		shellRun(`echo '[{"name":"a","n":9007199254740993},{"name":"b","metadata":{"zone":"z"}}]'; echo '{"name":"c"}'`, set),
	}}
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.Len(t, res.Records, 3)
	encoded, err := json.Marshal(res.Records)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"name":"a","n":9007199254740993,"metadata":{"labels":{"env":"prod"}}},
		{"name":"b","metadata":{"zone":"z","labels":{"env":"prod"}}},
		{"name":"c","metadata":{"labels":{"env":"prod"}}}
	]`, string(encoded))
}

// A failing run is a failed input: the other runs' records are kept, and the
// failure carries its argv, exit status and stderr tail.
func TestRunRecordsFailedInputsAndKeepsTheRest(t *testing.T) {
	var stderr bytes.Buffer
	p := systemmodel.Populate{Runs: []systemmodel.CommandRun{
		shellRun(`echo '{"name":"a"}'`, nil),
		shellRun(`echo boom >&2; exit 3`, nil),
		shellRun(`echo '[1]'`, nil),
		shellRun(`echo '{"name":"d"}'`, nil),
	}}
	res, err := Run(context.Background(), p, Request{Stderr: &stderr})
	require.NoError(t, err)
	assert.Equal(t, 4, res.Runs)
	require.Len(t, res.Records, 2)
	require.Len(t, res.Failed, 2)

	exited := res.Failed[0]
	assert.Equal(t, 1, exited.Run)
	assert.Equal(t, []string{"sh", "-c", "echo boom >&2; exit 3"}, exited.Argv)
	assert.Equal(t, 3, exited.ExitStatus)
	assert.Equal(t, "exit status 3", exited.Error)
	assert.Equal(t, "boom", exited.Stderr)
	assert.Equal(t, "boom\n", stderr.String(), "stderr passes through as the command runs")

	undecodable := res.Failed[1]
	assert.Equal(t, 2, undecodable.Run)
	assert.Equal(t, 0, undecodable.ExitStatus)
	assert.Equal(t, "stdout record 0 is json.Number, not a JSON object", undecodable.Error)
}

// fail_fast keeps all-or-nothing: the first failure is the populate's error,
// quoting the end of its stderr, and later runs never start.
func TestRunFailFastStopsAtTheFirstFailure(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	p := systemmodel.Populate{FailFast: true, Runs: []systemmodel.CommandRun{
		shellRun(`echo boom >&2; exit 3`, nil),
		shellRun(`touch "`+marker+`"; echo '{}'`, nil),
	}}
	_, err := Run(context.Background(), p, Request{})
	require.ErrorContains(t, err, "populate run 0 (sh -c echo boom >&2; exit 3): exit status 3")
	require.ErrorContains(t, err, "stderr (last 4096 bytes):\nboom")
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr))

	_, err = Run(context.Background(), systemmodel.Populate{FailFast: true, Runs: []systemmodel.CommandRun{shellRun(`echo '[1]'`, nil)}}, Request{})
	require.ErrorContains(t, err, "stdout record 0 is json.Number, not a JSON object")
}

// An interrupt is not a failed input: it stops the populate.
func TestRunStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '{}'`, nil)}}, Request{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunBoundsSpooledOutput(t *testing.T) {
	p := systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '[{"name":"a"}]'`, nil)}}
	_, err := Run(context.Background(), p, Request{Limits: ingestprep.Limits{StagingBytes: 4}})
	require.Error(t, err)
}

func TestRunReleasesItsSpoolDirectory(t *testing.T) {
	var tracked string
	released := false
	req := Request{TrackTempDir: func(dir string) func() {
		tracked = dir
		return func() { released = true; _ = os.RemoveAll(dir) }
	}}
	_, err := Run(context.Background(), systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '{}'`, nil)}}, req)
	require.NoError(t, err)
	require.NotEmpty(t, tracked)
	assert.True(t, released)
	_, statErr := os.Stat(tracked)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRunResolvesRelativeProgramsAgainstTheModelDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "emit.sh"), []byte("#!/bin/sh\necho '{\"ok\":true}'\n"), 0o755))
	p := systemmodel.Populate{Runs: []systemmodel.CommandRun{{Argv: []string{"./emit.sh"}}}}
	res, err := Run(context.Background(), p, Request{ModelDir: dir})
	require.NoError(t, err)
	require.Len(t, res.Records, 1)
}

func TestRunRejectsABadTimeout(t *testing.T) {
	_, err := Run(context.Background(), systemmodel.Populate{Timeout: "soon"}, Request{})
	require.ErrorContains(t, err, "populate timeout")
}
