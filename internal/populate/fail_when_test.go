package populate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

// A run that exits 0 with stderr matching fail_when is a failed input with
// exit status 0, and its records are discarded; a run whose stderr matches
// nothing keeps its records. A fail_when match is never an empty input.
func TestRunFailWhenFailsARunThatExitedZero(t *testing.T) {
	p := systemmodel.Populate{
		FailWhen:  []string{"Some requests did not succeed"},
		EmptyWhen: []string{"did not succeed"},
		Runs: []systemmodel.CommandRun{
			shellRun(`echo '{"name":"a"}'; echo 'WARNING: all good' >&2`, nil),
			shellRun(`echo '[]'; printf "WARNING: Some requests did not succeed.\n - Required 'compute.addresses.list' permission\n" >&2`, nil),
			shellRun(`echo '{"name":"partial"}'; echo 'WARNING: Some requests did not succeed.' >&2`, nil),
		},
	}
	res, err := Run(context.Background(), p, Request{})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Runs)
	require.Len(t, res.Records, 1, "only the run whose stderr matched nothing")
	assert.Empty(t, res.Empty)
	require.Len(t, res.Failed, 2)
	f := res.Failed[0]
	assert.Equal(t, 1, f.Run)
	assert.Equal(t, 0, f.ExitStatus)
	assert.Contains(t, f.Error, `matched fail_when "Some requests did not succeed": WARNING: Some requests did not succeed.`)
	assert.Contains(t, f.Stderr, "compute.addresses.list")
	assert.Equal(t, 2, res.Failed[1].Run, "its record is discarded")
}

// fail_when trips fail_fast like any failure.
func TestRunFailWhenTripsFailFast(t *testing.T) {
	p := systemmodel.Populate{FailFast: true, FailWhen: []string{"did not succeed"}, Runs: []systemmodel.CommandRun{
		shellRun(`echo '[]'; echo 'Some requests did not succeed' >&2`, nil),
	}}
	_, err := Run(context.Background(), p, Request{})
	require.ErrorContains(t, err, "matched fail_when")
}

// A bad pattern stops the populate before anything runs.
func TestRunFailWhenRejectsABadPattern(t *testing.T) {
	p := systemmodel.Populate{FailWhen: []string{"(unclosed"}, Runs: []systemmodel.CommandRun{shellRun(`echo '[]'`, nil)}}
	_, err := Run(context.Background(), p, Request{})
	require.ErrorContains(t, err, `populate.fail_when[0] "(unclosed" is not a valid regular expression`)
}
