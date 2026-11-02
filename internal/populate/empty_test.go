package populate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

// A run that exits non-zero with stderr matching empty_when is an empty input
// with the matched pattern and line; a failure no pattern matches is still a
// failed input, and so is one that printed undecodable stdout.
func TestRunRecordsEmptyInputsSeparately(t *testing.T) {
	p := systemmodel.Populate{EmptyWhen: []string{"SERVICE_DISABLED"}, Runs: []systemmodel.CommandRun{
		shellRun(`echo '{"name":"a"}'`, nil),
		shellRun(`printf 'ERROR: API disabled\n  reason: SERVICE_DISABLED\n' >&2; exit 1`, nil),
		shellRun(`echo 'reason: PERMISSION_DENIED' >&2; exit 1`, nil),
		shellRun(`echo SERVICE_DISABLED >&2; echo '[1]'`, nil),
	}}
	res, err := Run(context.Background(), p, Request{})
	require.NoError(t, err)
	assert.Equal(t, 4, res.Runs)
	assert.Len(t, res.Records, 1)

	require.Len(t, res.Empty, 1)
	empty := res.Empty[0]
	assert.Equal(t, 1, empty.Run)
	assert.Equal(t, 1, empty.ExitStatus)
	assert.Equal(t, "SERVICE_DISABLED", empty.Pattern)
	assert.Equal(t, "reason: SERVICE_DISABLED", empty.Reason)
	assert.Equal(t, "sh -c printf 'ERROR: API disabled\\n  reason: SERVICE_DISABLED\\n' >&2; exit 1", empty.Label())

	require.Len(t, res.Failed, 2)
	assert.Equal(t, 2, res.Failed[0].Run, "an unmatched failure stays failed")
	assert.Equal(t, 3, res.Failed[1].Run, "undecodable stdout is never empty")
}

// A timeout is never an empty input, whatever its stderr says.
func TestRunTimeoutIsNeverEmpty(t *testing.T) {
	p := systemmodel.Populate{Timeout: "200ms", EmptyWhen: []string{"SERVICE_DISABLED"}, Runs: []systemmodel.CommandRun{
		shellRun(`echo SERVICE_DISABLED >&2; trap 'kill $!; exit 1' TERM; sleep 5 & wait`, nil),
	}}
	res, err := Run(context.Background(), p, Request{})
	require.NoError(t, err)
	assert.Empty(t, res.Empty)
	require.Len(t, res.Failed, 1)
	assert.Equal(t, 1, res.Failed[0].ExitStatus, "it exited 1 on SIGTERM; the deadline, not the status, rules it out")
}

// An empty input does not trip fail_fast: it is not a failure.
func TestRunFailFastIgnoresEmptyInputs(t *testing.T) {
	p := systemmodel.Populate{FailFast: true, EmptyWhen: []string{"SERVICE_DISABLED"}, Runs: []systemmodel.CommandRun{
		shellRun(`echo SERVICE_DISABLED >&2; exit 1`, nil),
		shellRun(`echo '{"name":"b"}'`, nil),
	}}
	res, err := Run(context.Background(), p, Request{})
	require.NoError(t, err)
	assert.Len(t, res.Empty, 1)
	assert.Len(t, res.Records, 1)
}

func TestRunRejectsABadEmptyWhenPattern(t *testing.T) {
	_, err := Run(context.Background(), systemmodel.Populate{EmptyWhen: []string{"("}}, Request{})
	require.ErrorContains(t, err, "populate.empty_when[0]")
}

func TestLineAt(t *testing.T) {
	s := "first\n  second line \nthird"
	assert.Equal(t, "first", lineAt(s, 2))
	assert.Equal(t, "second line", lineAt(s, 9))
	assert.Equal(t, "third", lineAt(s, len(s)-1))
}
