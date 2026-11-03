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

func withFallback(primary systemmodel.Populate, when []string, fb systemmodel.Populate) systemmodel.Populate {
	primary.Fallback = &systemmodel.Fallback{When: when, Observe: fb}
	return primary
}

func recordsJSON(t *testing.T, res Result) string {
	t.Helper()
	b, err := json.Marshal(res.Records)
	require.NoError(t, err)
	return string(b)
}

// A run failing with a matching error abandons the populate's runs: earlier
// records are discarded, later runs never start, and the fallback answers.
func TestFallbackReplacesThePopulateOnAMatchingFailure(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "later-run-ran")
	var stderr bytes.Buffer
	p := withFallback(systemmodel.Populate{Runs: []systemmodel.CommandRun{
		shellRun(`echo '{"name":"from-primary"}'`, nil),
		shellRun(`echo 'ERROR: reason: SERVICE_DISABLED for the asset API' >&2; exit 1`, nil),
		shellRun(`touch `+marker+`; echo '{"name":"late"}'`, nil),
	}}, []string{"SERVICE_DISABLED"}, systemmodel.Populate{Runs: []systemmodel.CommandRun{
		shellRun(`echo '{"name":"from-fallback"}'`, map[string]any{"source": "gcloud"}),
	}})
	res, err := Run(context.Background(), p, Request{ModelDir: dir, Stderr: &stderr})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"from-fallback","source":"gcloud"}]`, recordsJSON(t, res))
	assert.NoFileExists(t, marker, "runs after the trigger are skipped")
	require.NotNil(t, res.Fallback)
	assert.Equal(t, FallbackFailedInput, res.Fallback.Reason)
	assert.Equal(t, "SERVICE_DISABLED", res.Fallback.Pattern)
	assert.Equal(t, "ERROR: reason: SERVICE_DISABLED for the asset API", res.Fallback.StderrLine)
	require.NotNil(t, res.Fallback.Trigger)
	assert.Equal(t, 1, res.Fallback.Trigger.Run)
	assert.Equal(t, 1, res.Runs, "the counts are the fallback's")
	assert.Empty(t, res.Failed)
	assert.Contains(t, stderr.String(), "using the populate fallback")
}

// The fallback check comes before empty_when: an unavailable source is not an
// empty one.
func TestFallbackWinsOverEmptyWhen(t *testing.T) {
	p := withFallback(systemmodel.Populate{
		EmptyWhen: []string{"SERVICE_DISABLED"},
		Runs:      []systemmodel.CommandRun{shellRun(`echo SERVICE_DISABLED >&2; exit 1`, nil)},
	}, []string{"SERVICE_DISABLED"}, systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '{"name":"f"}'`, nil)}})
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.NotNil(t, res.Fallback)
	assert.Empty(t, res.Empty)
	assert.Len(t, res.Records, 1)
}

// A failure that matches no pattern is an ordinary failed input.
func TestFallbackIgnoresOtherFailures(t *testing.T) {
	p := withFallback(systemmodel.Populate{Runs: []systemmodel.CommandRun{
		shellRun(`echo boom >&2; exit 2`, nil),
		shellRun(`echo '{"name":"a"}'`, nil),
	}}, []string{"SERVICE_DISABLED"}, systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '{"name":"f"}'`, nil)}})
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	assert.Nil(t, res.Fallback)
	require.Len(t, res.Failed, 1)
	assert.JSONEq(t, `[{"name":"a"}]`, recordsJSON(t, res))
}

// A timeout never triggers the fallback: only a command's own error output
// says the source is unavailable.
func TestFallbackNeedsAnExitedCommand(t *testing.T) {
	p := withFallback(systemmodel.Populate{Timeout: "200ms", Runs: []systemmodel.CommandRun{
		shellRun(`echo SERVICE_DISABLED >&2; exec sleep 5`, nil),
	}}, []string{"SERVICE_DISABLED"}, systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '{"name":"f"}'`, nil)}})
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	assert.Nil(t, res.Fallback)
	require.Len(t, res.Failed, 1)
}

// A failing each with a matching error, and an each that lists nothing, both
// hand over to the fallback, which fans out with its own each.
func TestFallbackOnEachFailureAndNoInputs(t *testing.T) {
	fb := systemmodel.Populate{
		Each: &systemmodel.Enumeration{Argv: []string{"echo", `["p1","p2"]`}},
		Runs: []systemmodel.CommandRun{{Argv: []string{"sh", "-c", `echo "{\"name\":\"$0\"}"`, "{{item}}"}, Set: map[string]any{"project": "{{item}}"}}},
	}
	primaryRun := []systemmodel.CommandRun{{Argv: []string{"echo", `{"scope":"{{item}}"}`}}}
	for name, each := range map[string][]string{
		"each failed": {"sh", "-c", `echo 'PERMISSION_DENIED: organizations.list' >&2; exit 1`},
		"no inputs":   {"echo", `[]`},
	} {
		t.Run(name, func(t *testing.T) {
			p := withFallback(systemmodel.Populate{Each: &systemmodel.Enumeration{Argv: each}, Runs: primaryRun},
				[]string{"PERMISSION_DENIED"}, fb)
			res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
			require.NoError(t, err)
			require.NotNil(t, res.Fallback)
			assert.JSONEq(t, `[{"name":"p1","project":"p1"},{"name":"p2","project":"p2"}]`, recordsJSON(t, res))
			assert.Equal(t, []string{"p1", "p2"}, res.Inputs)
			if name == "no inputs" {
				assert.Equal(t, FallbackNoInputs, res.Fallback.Reason)
				assert.Nil(t, res.Fallback.Trigger)
			} else {
				assert.Equal(t, FallbackEachFailed, res.Fallback.Reason)
				assert.Equal(t, "PERMISSION_DENIED: organizations.list", res.Fallback.StderrLine)
			}
		})
	}
}

// An each that fails without a matching error still fails the populate.
func TestFallbackKeepsUnmatchedEachFailures(t *testing.T) {
	p := withFallback(systemmodel.Populate{
		Each: &systemmodel.Enumeration{Argv: []string{"sh", "-c", `echo nope >&2; exit 1`}},
		Runs: []systemmodel.CommandRun{{Argv: []string{"echo", "{{item}}"}}},
	}, []string{"PERMISSION_DENIED"}, systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo '{}'`, nil)}})
	_, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

// The fallback's own failures are failed inputs; its empty_when applies.
func TestFallbackFailuresAreFailedInputs(t *testing.T) {
	p := withFallback(systemmodel.Populate{Runs: []systemmodel.CommandRun{shellRun(`echo SERVICE_DISABLED >&2; exit 1`, nil)}},
		[]string{"SERVICE_DISABLED"}, systemmodel.Populate{
			EmptyWhen: []string{"API not enabled"},
			Runs: []systemmodel.CommandRun{
				shellRun(`echo '{"name":"a"}'`, nil),
				shellRun(`echo broken >&2; exit 1`, nil),
				shellRun(`echo 'API not enabled' >&2; exit 1`, nil),
			},
		})
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir()})
	require.NoError(t, err)
	require.NotNil(t, res.Fallback)
	assert.Len(t, res.Records, 1)
	assert.Len(t, res.Failed, 1)
	assert.Len(t, res.Empty, 1)
}

// The abandoned runs' spool does not count against the fallback's budget.
func TestFallbackGetsAFreshStagingBudget(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.json")
	payload := bytes.Repeat([]byte(`{"name":"padding-padding-padding"}`+"\n"), 2000)
	require.NoError(t, os.WriteFile(big, payload, 0o600))
	limits := ingestprep.Limits{StagingBytes: int64(len(payload)) + 100}
	p := withFallback(systemmodel.Populate{Runs: []systemmodel.CommandRun{
		{Argv: []string{"cat", big}},
		shellRun(`echo SERVICE_DISABLED >&2; exit 1`, nil),
	}}, []string{"SERVICE_DISABLED"}, systemmodel.Populate{Runs: []systemmodel.CommandRun{{Argv: []string{"cat", big}}}})
	res, err := Run(context.Background(), p, Request{ModelDir: t.TempDir(), Limits: limits})
	require.NoError(t, err)
	assert.Len(t, res.Records, 2000)
}
