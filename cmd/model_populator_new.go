package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var modelPopulatorNewForce bool

var modelPopulatorNewCmd = &cobra.Command{
	Use:   "new <model>",
	Short: "Scaffold an #EweTarget populator",
	Long: `Create a minimal ewe populator under populators/<model>/.
The generated program writes a records array and already uses a quoted
"_schema" field, so its output can be ingested by pudl without the hidden-CUE
field trap. Edit the TODO before running the model.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := targetPudlRoot(false)
		if err != nil {
			return err
		}
		dir := filepath.Join(root, "populators", args[0])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create populator directory: %w", err)
		}
		path := filepath.Join(dir, "populate.cue")
		if !modelPopulatorNewForce {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("populator already exists: %s (use --force to replace)", path)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("check populator: %w", err)
			}
		}
		const source = `// Generated populator for MODEL. Replace the placeholder fetch with
// an op.#HttpAll, command, or other effect and keep the quoted "_schema" tag.
import "op"

_env: op.#Env & { args: [] }

_records: [{
	"_schema": "replace.resource"
	// TODO: add fields from the upstream response.
}]

write: op.#WriteFile & { args: ["\(_env.result.MU_OUT)/records.json", _records] }
`
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			return fmt.Errorf("write populator: %w", err)
		}
		result := map[string]any{"path": path, "ewe_source": filepath.Join(args[0], "populate.cue"), "model": args[0]}
		if jsonOutput {
			b, _ := json.Marshal(result)
			fmt.Println(string(b))
		} else {
			fmt.Printf("created populator scaffold: %s\n", path)
			fmt.Printf("set populate.eweSource to %q in the model\n", filepath.Join(args[0], "populate.cue"))
		}
		return nil
	},
}

func init() {
	modelPopulatorNewCmd.Flags().BoolVar(&modelPopulatorNewForce, "force", false, "Replace an existing populator scaffold")
}
