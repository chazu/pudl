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
	records, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.Len(t, records, 3)
	encoded, err := json.Marshal(records)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"name":"a","n":9007199254740993,"metadata":{"labels":{"env":"prod"}}},
		{"name":"b","metadata":{"zone":"z","labels":{"env":"prod"}}},
		{"name":"c","metadata":{"labels":{"env":"prod"}}}
	]`, string(encoded))
}

func TestRunFailsOnNonObjectRecordAndQuotesStderr(t *testing.T) {
	_, err := Run(context.Background(), systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '[1]'`, nil)}}, Request{})
	require.ErrorContains(t, err, "stdout record 0 is json.Number, not a JSON object")

	var stderr bytes.Buffer
	_, err = Run(context.Background(), systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo boom >&2; exit 3`, nil)}}, Request{Stderr: &stderr})
	require.ErrorContains(t, err, "populate run 0 (sh -c echo boom >&2; exit 3)")
	require.ErrorContains(t, err, "stderr (last 4096 bytes):\nboom")
	assert.Equal(t, "boom\n", stderr.String(), "stderr passes through as the command runs")
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
	records, err := Run(context.Background(), p, Request{ModelDir: dir})
	require.NoError(t, err)
	require.Len(t, records, 1)
}

func TestRunRejectsABadTimeout(t *testing.T) {
	_, err := Run(context.Background(), systemmodel.Populate{Timeout: "soon"}, Request{})
	require.ErrorContains(t, err, "populate timeout")
}
