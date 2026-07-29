package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var ruleNewForce bool

var ruleNewCmd = &cobra.Command{
	Use:          "new <name>",
	Short:        "Scaffold a Datalog rule",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := wsPolicy.RuleWritePath()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create rules directory: %w", err)
		}
		fileName := modelFileName(args[0])
		path := filepath.Join(dir, fileName)
		if !ruleNewForce {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("rule file already exists: %s (use --force to replace)", path)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("check rule file: %w", err)
			}
		}
		definition := modelDefinitionName(args[0])
		source := fmt.Sprintf(`package rules

import r "pudl.schemas/pudl/rules@v0"

#%s: r.#Rule & {
	name: %q
	head: {
		rel: "derived_relation"
		args: {X: "$X"}
	}
	body: [{
		rel: "source_relation"
		args: {X: "$X"}
	}]
}
`, definition, args[0])
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			return fmt.Errorf("write rule scaffold: %w", err)
		}
		if jsonOutput {
			b, _ := json.Marshal(map[string]any{"path": path, "name": strings.TrimSpace(args[0])})
			fmt.Println(string(b))
		} else {
			fmt.Printf("created rule scaffold: %s\n", path)
		}
		return nil
	},
}

func init() {
	ruleNewCmd.Flags().BoolVar(&ruleNewForce, "force", false, "Replace an existing rule scaffold")
}
