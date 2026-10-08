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

--populate takes plugin:<name> (a cached mu plugin) or command:<cmdline>, a
command printing JSON records, run directly by pudl (no shell, no mu).

Examples:
    pudl model new pods --populate plugin:k8s --input namespace=default
    pudl model new gcp-firewalls --populate 'command:gcloud compute firewall-rules list --format=json'`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := parsePopulateSpec(modelNewPopulate)
		if err != nil {
			return err
		}
		input, err := parseKeyValueInputs(modelNewInputs)
		if err != nil {
			return err
		}
		path, err := writeModelScaffold(args[0], spec, input, false, modelNewForce)
		if err != nil {
			return err
		}
		if jsonOutput {
			out := map[string]any{"path": path, "name": args[0]}
			if spec.argv != nil {
				out["command"] = spec.argv
			} else {
				out["plugin"] = spec.plugin
			}
			b, _ := json.Marshal(out)
			fmt.Fprintln(outw(), string(b))
			return nil
		}
		fmt.Fprintf(outw(), "created model scaffold: %s\n", path)
		fmt.Fprintf(outw(), "next: pudl model show %s\n", args[0])
		return nil
	},
}

func init() {
	modelNewCmd.Flags().StringVar(&modelNewPopulate, "populate", "", "Populate arm: plugin:<name> or command:<cmdline>")
	modelNewCmd.Flags().StringArrayVar(&modelNewInputs, "input", nil, "Populate input key=value (repeatable; JSON values are decoded)")
	modelNewCmd.Flags().BoolVar(&modelNewForce, "force", false, "Replace an existing scaffold file")
}
