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
global schema outside a workspace). The scaffold observes with #CommandObserve:
commands that print JSON records, run directly by pudl (no shell). Always edit
the returned file to add desired state or checks.

--populate command:<cmdline> sets the command. Without it the scaffold runs a
placeholder command that prints no records; replace it before relying on it.

Examples:
    pudl model new gcp-firewalls --populate 'command:gcloud compute firewall-rules list --format=json'
    pudl model new inventory`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if modelNewCheck != "" {
			return newCheckModel(cmd, args[0])
		}
		if len(modelNewEvidence) > 0 || modelNewMaxAge != "" || cmd.Flags().Changed("expect") {
			return fmt.Errorf("--evidence, --max-age and --expect require --check")
		}
		var argv []string
		if modelNewPopulate != "" {
			spec, err := parsePopulateSpec(modelNewPopulate)
			if err != nil {
				return err
			}
			argv = spec.argv
		}
		path, err := writeModelScaffold(args[0], argv, false, modelNewForce)
		if err != nil {
			return err
		}
		if jsonOutput {
			if argv == nil {
				argv = defaultScaffoldArgv
			}
			b, _ := json.Marshal(map[string]any{"path": path, "name": args[0], "command": argv})
			fmt.Fprintln(outw(), string(b))
			return nil
		}
		fmt.Fprintf(outw(), "created model scaffold: %s\n", path)
		fmt.Fprintf(outw(), "next: pudl model show %s\n", args[0])
		return nil
	},
}

func init() {
	modelNewCmd.Flags().StringVar(&modelNewPopulate, "populate", "", "Populate command: command:<cmdline> (default: a placeholder printing no records)")
	modelNewCmd.Flags().BoolVar(&modelNewForce, "force", false, "Replace an existing scaffold file")
}
