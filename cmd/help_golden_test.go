package cmd

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateHelpGolden = flag.Bool("update", false, "rewrite testdata/help.golden.json from the current command tree")

// TestHelpJSONGolden pins the machine-readable command tree. Any change to the
// CLI surface — a command, flag, default or description — shows up as a diff
// to testdata/help.golden.json in review. Regenerate with:
//
//	go test ./cmd -run TestHelpJSONGolden -update
func TestHelpJSONGolden(t *testing.T) {
	golden, err := filepath.Abs(filepath.Join("testdata", "help.golden.json"))
	require.NoError(t, err)

	// Execute in an isolated workspace: running the root command resolves the
	// workspace policy from the working directory, which other tests rely on.
	cliWorkspace(t)
	r := runCLI(t, "help", "--json")
	require.NoError(t, r.Err, "stderr=%s", r.Stderr)

	if *updateHelpGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0o755))
		require.NoError(t, os.WriteFile(golden, []byte(r.Stdout), 0o644))
		return
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err, "missing golden file; run with -update")
	require.Equal(t, string(want), r.Stdout,
		"`pudl help --json` changed; if intended, run: go test ./cmd -run TestHelpJSONGolden -update")
}
