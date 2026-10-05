package ui

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Streams separates a command's result output from its diagnostics. Results
// (text reports or JSON documents) go to Out; warnings, notes and progress go to
// Err, so a caller can parse Out without stripping prose.
type Streams struct {
	Out io.Writer
	Err io.Writer
}

// FromCmd returns the streams a cobra command is configured with.
func FromCmd(c *cobra.Command) Streams {
	return Streams{Out: c.OutOrStdout(), Err: c.ErrOrStderr()}
}

// Warnf writes a warning line to Err.
func (s Streams) Warnf(format string, args ...any) {
	fmt.Fprintf(s.Err, "Warning: "+format+"\n", args...)
}

// Progressf writes a progress or status line to Err.
func (s Streams) Progressf(format string, args ...any) {
	fmt.Fprintf(s.Err, format+"\n", args...)
}
