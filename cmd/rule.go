package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/datalog"
)

var (
	ruleGlobal bool
)

var ruleCmd = &cobra.Command{
	Use:   "rule",
	Short: "Manage Datalog rules",
	Long: `Manage Datalog rules used by pudl query.

Rules are CUE files stored in:
  .pudl/schema/pudl/rules/     (repo-scoped)
  ~/.pudl/schema/pudl/rules/   (global)

Available subcommands:
- add: Install a rule file into the workspace

Examples:
    pudl rule add my-rules.cue
    pudl rule add my-rules.cue --global`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var ruleAddCmd = &cobra.Command{
	Use:   "add <file>",
	Short: "Install a Datalog rule file into the workspace",
	Long: `Validate and install a CUE rule file into the rule directory.

The file must contain valid CUE with at least one #Rule-shaped value
(fields with head and body). The file is validated before installation.

By default, rules are installed repo-scoped (.pudl/schema/pudl/rules/).
Use --global to install to ~/.pudl/schema/pudl/rules/.

Examples:
    pudl rule add transitive-deps.cue
    pudl rule add company-standards.cue --global`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		srcPath := args[0]

		// Read source file
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", srcPath, err)
		}

		// Validate: parse as CUE and extract rules
		rules, err := datalog.ParseRulesFromSource(string(data))
		if err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
		if len(rules) == 0 {
			return fmt.Errorf("no rules found in %s — file must contain at least one field with head and body", srcPath)
		}

		// Determine target directory
		var targetDir string
		if ruleGlobal {
			targetDir = filepath.Join(config.GetPudlDir(), "schema", "pudl", "rules")
		} else {
			if wsCtx == nil || wsCtx.Workspace == nil {
				return fmt.Errorf("not in a workspace — use --global or run from a repo with .pudl/")
			}
			targetDir = filepath.Join(wsCtx.Workspace.PudlDir, "schema", "pudl", "rules")
		}

		// Ensure directory exists
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create rules directory: %w", err)
		}

		// Copy file
		fileName := filepath.Base(srcPath)
		if !strings.HasSuffix(fileName, ".cue") {
			fileName += ".cue"
		}
		targetPath := filepath.Join(targetDir, fileName)

		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write rule file: %w", err)
		}

		// Report
		scope := "repo-scoped"
		if ruleGlobal {
			scope = "global"
		}

		fmt.Printf("Installed %d rule(s) from %s (%s)\n", len(rules), fileName, scope)
		for _, r := range rules {
			name := r.Name
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Printf("  %s: %s :- %s\n", name, r.Head.Rel, ruleBodySummary(r))
		}
		fmt.Printf("Location: %s\n", targetPath)

		return nil
	},
}

func ruleBodySummary(r datalog.Rule) string {
	var rels []string
	for _, a := range r.Body {
		rels = append(rels, a.Rel)
	}
	return strings.Join(rels, ", ")
}

func init() {
	rootCmd.AddCommand(ruleCmd)
	ruleCmd.AddCommand(ruleAddCmd)

	ruleAddCmd.Flags().BoolVar(&ruleGlobal, "global", false, "Install as a global rule (~/.pudl/schema/pudl/rules/)")
}
