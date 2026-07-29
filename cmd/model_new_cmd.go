package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var modelNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Scaffold a #SystemModel observer",
	Long: `Create a registered #SystemModel scaffold in the project schema (or the
global schema outside a workspace). Always edit the returned file to add desired
state, checks, or a converge arm.

Example:
    pudl model new pods --populate plugin:k8s --input namespace=default`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		plugin, err := parsePluginSpec(modelNewPopulate)
		if err != nil {
			return err
		}
		input, err := parseKeyValueInputs(modelNewInputs)
		if err != nil {
			return err
		}
		path, err := writeModelScaffold(args[0], plugin, input, false, modelNewForce)
		if err != nil {
			return err
		}
		if jsonOutput {
			b, _ := json.Marshal(map[string]any{"path": path, "name": args[0], "plugin": plugin})
			fmt.Println(string(b))
			return nil
		}
		fmt.Printf("created model scaffold: %s\n", path)
		fmt.Printf("next: pudl model show %s\n", args[0])
		return nil
	},
}

func init() {
	modelNewCmd.Flags().StringVar(&modelNewPopulate, "populate", "", "Populate arm, in the form plugin:<name>")
	modelNewCmd.Flags().StringArrayVar(&modelNewInputs, "input", nil, "Populate input key=value (repeatable; JSON values are decoded)")
	modelNewCmd.Flags().BoolVar(&modelNewForce, "force", false, "Replace an existing scaffold file")
}
