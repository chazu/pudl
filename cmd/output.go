package cmd

import "io"

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
