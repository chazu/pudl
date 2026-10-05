package cmd

import (
	"io"

	"github.com/spf13/cobra"

	pudlerrors "github.com/chazu/pudl/internal/errors"
)

// outw is the writer for a command's result. It is the root command's
// configured output (os.Stdout unless a caller or test used SetOut), so every
// subcommand writes results to the same injectable stream.
func outw() io.Writer { return rootCmd.OutOrStdout() }

// errw is the writer for diagnostics: warnings, notes, and progress. Keeping
// these off outw keeps --json output parseable.
func errw() io.Writer { return rootCmd.ErrOrStderr() }

// printJSON writes v as one indented JSON document on the result stream.
func printJSON(v any) error {
	return GetOutputWriter().WriteJSON(v)
}

// pudlRunE adapts a command body to cobra's RunE. A failure is reported on the
// error stream the way the CLI error handler always has — the message plus any
// suggestions — and returned, so Execute chooses the exit code (a PUDLError's
// own code) instead of the command exiting mid-run.
func pudlRunE(body func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := body(cmd, args)
		if err != nil {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			pudlerrors.Display(errw(), err)
		}
		return err
	}
}
