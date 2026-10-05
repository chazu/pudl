package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	pudlerrors "github.com/chazu/pudl/internal/errors"
)

// A command failure is reported once on stderr, with its suggestions, and
// returned for Execute to map to the error's exit code — not os.Exit'd from
// inside the command.
func TestPudlRunEReportsErrorsOnceWithSuggestions(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "show", "babab-babab")
	require.Error(t, r.Err)
	require.Empty(t, r.Stdout, "a failure writes nothing to the result stream")
	require.Equal(t, 1, strings.Count(r.Stderr, "Error:"), "cobra must not repeat the error:\n%s", r.Stderr)
	require.Contains(t, r.Stderr, "Suggestions:")
	require.NotContains(t, r.Stderr, "Usage:", "runtime failures do not print usage")

	var pudlErr *pudlerrors.PUDLError
	require.True(t, errors.As(r.Err, &pudlErr))
	require.Equal(t, pudlErr.GetExitCode(), exitCodeFor(r.Err))
}

// Diagnostics never reach the result stream: a JSON consumer can parse stdout
// while warnings go to stderr.
func TestDiagnosticsStayOffStdout(t *testing.T) {
	cliWorkspace(t)
	t.Chdir(t.TempDir()) // no workspace here, and the global one is uninitialized
	r := runCLI(t, "config", "--json")
	require.NoError(t, r.Err)
	require.True(t, json.Valid([]byte(r.Stdout)), r.Stdout)
	require.Contains(t, r.Stderr, "Workspace not initialized")
	require.NotContains(t, r.Stdout, "Workspace not initialized")
}
