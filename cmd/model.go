package cmd

import (
	"github.com/spf13/cobra"
)

var (
	modelVerbose  bool
	modelCategory string
)

// modelCmd represents the model command
var modelCmd = &cobra.Command{
	Use:     "model",
	Aliases: []string{"m"},
	Short:   "Manage models (schemas with operational capabilities)",
	Long: `Manage models that compose CUE schemas with operational behavior.

A model references one or more schemas and adds methods, sockets,
authentication, and metadata. Schemas remain pure data shapes — models
layer behavior on top.

Available subcommands:
- list:    Show available models
- show:    Display model details

Examples:
    pudl model list
    pudl model list --category compute
    pudl model show pudl/model/examples.#EC2InstanceModel`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(modelCmd)
}
