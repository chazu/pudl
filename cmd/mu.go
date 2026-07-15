package cmd

import (
	"github.com/spf13/cobra"
)

// muCmd groups the mu-bridge commands. mu is the execution layer ("pudl knows,
// mu acts"); these subcommands move data across the boundary: rendering desired
// state through a model run, and ingesting mu's observe/build results back into the
// catalog. Keeping them under one namespace separates the mu bridge from the
// fact/agent-memory door (`pudl facts`) and the data-lake door (`pudl import`).
var muCmd = &cobra.Command{
	Use:   "mu",
	Short: "Bridge commands between pudl and the mu execution layer",
	Long: `Commands that move data across the pudl/mu boundary.

pudl knows; mu acts. These subcommands ingest mu's execution results back into
the catalog.

Available subcommands:
- ingest-observe:  Store mu observe results as live catalog state
- ingest-manifest: Record a mu build manifest in the catalog

Examples:
    mu observe --json //home/odroid | pudl mu ingest-observe
    mu build --emit-manifest | pudl mu ingest-manifest`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(muCmd)
	muCmd.AddCommand(ingestObserveCmd)
	muCmd.AddCommand(ingestManifestCmd)
}
