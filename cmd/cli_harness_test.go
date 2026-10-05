package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/repo"
	"github.com/chazu/pudl/internal/validator"
)

// cliResult is what one in-process pudl invocation wrote and returned.
type cliResult struct {
	Stdout string
	Stderr string
	Err    error
}

// runCLI executes the root command in-process with args, capturing stdout and
// stderr separately. Flag values bound to package variables persist between
// cobra executions, so every flag in the tree is reset to its default first.
func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	resetCommandFlags(rootCmd)
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		resetCommandFlags(rootCmd)
	}()
	err := rootCmd.Execute()
	return cliResult{Stdout: out.String(), Stderr: errOut.String(), Err: err}
}

// resetCommandFlags restores every flag on c and its descendants to its
// declared default and clears its Changed bit.
func resetCommandFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(parseSliceDefault(f.DefValue))
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetCommandFlags(sub)
	}
}

func parseSliceDefault(def string) []string {
	def = strings.TrimSuffix(strings.TrimPrefix(def, "["), "]")
	if def == "" {
		return []string{}
	}
	return strings.Split(def, ",")
}

// cliWorkspace creates an isolated repository workspace (and HOME) and makes
// it the working directory for in-process CLI invocations.
func cliWorkspace(t *testing.T) string {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(root)
	require.NoError(t, repo.Init(repo.InitOptions{Dir: root}))
	previous := wsPolicy
	resetShared := func() {
		inference.ResetShared()
		validator.ResetSharedLoaders()
	}
	resetShared()
	t.Cleanup(func() {
		wsPolicy = previous
		resetShared()
	})
	return filepath.Join(root, ".pudl")
}
