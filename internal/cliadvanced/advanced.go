// Package cliadvanced marks commands that stay out of everyday help but remain
// supported. A marked command is hidden from `--help` listings and shell
// completion; `pudl help --json` flags it as advanced, and the CLI reference
// documents it under "Advanced commands".
//
// The marker lives in its own package so that both cmd and internal/clidocs
// can read it without importing each other.
package cliadvanced

import "github.com/spf13/cobra"

const annotation = "pudl.advanced"

// Mark hides c from everyday help and marks it as an advanced command.
func Mark(c *cobra.Command) {
	c.Hidden = true
	if c.Annotations == nil {
		c.Annotations = map[string]string{}
	}
	c.Annotations[annotation] = "true"
}

// Is reports whether c was marked with Mark.
func Is(c *cobra.Command) bool {
	return c.Annotations[annotation] == "true"
}
