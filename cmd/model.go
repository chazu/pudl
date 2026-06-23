package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/workspace"
)

var modelPopulatorGlobal bool

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage #SystemModel definitions and their assets",
	Long: `Manage registered #SystemModel definitions and the assets they reference.

A #SystemModel is registered as a CUE definition inheriting #SystemModel in the
schema repository (project .pudl/schema or global ~/.pudl/schema). An
#EweTarget populate arm references a populator program (eweSource), stored under
the repo's populators/ directory — kept out of the schema CUE module so it is
not (mis)loaded as a schema.`,
}

var modelPopulatorCmd = &cobra.Command{
	Use:   "populator",
	Short: "Manage populator programs for #EweTarget models",
}

var modelPopulatorAddCmd = &cobra.Command{
	Use:   "add <model> <populator.cue>",
	Short: "Install a populator program into the repo for a model",
	Long: `Copy a populator (.cue) program into the pudl repo's populators/<model>/
directory so a registered #SystemModel can reference it as a repo-relative
eweSource. By default it goes in the project repo (.pudl) when one is found by
walking up from the cwd, else the global ~/.pudl; use --global to force global.

After installing, set the model's eweSource to the printed value.`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		model, srcFile := args[0], args[1]

		pudlRoot, err := targetPudlRoot(modelPopulatorGlobal)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(srcFile)
		if err != nil {
			return fmt.Errorf("read populator %q: %w", srcFile, err)
		}
		base := filepath.Base(srcFile)
		destDir := filepath.Join(pudlRoot, "populators", model)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return fmt.Errorf("create populators dir: %w", err)
		}
		dest := filepath.Join(destDir, base)
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("write populator: %w", err)
		}

		eweSource := filepath.Join(model, base) // repo-relative, under populators/
		fmt.Printf("installed populator: %s\n", dest)
		fmt.Printf("set in the model's populate arm:  eweSource: %q\n", eweSource)
		return nil
	},
}

// targetPudlRoot returns the .pudl repo to write into: the project repo (found
// by walking up for .pudl/workspace.cue) unless global is requested or no
// project repo exists, in which case the global ~/.pudl.
func targetPudlRoot(global bool) (string, error) {
	if !global {
		if ws, _ := workspace.Discover("."); ws != nil && ws.PudlDir != "" {
			return ws.PudlDir, nil
		}
	}
	root := config.GetPudlDir()
	if root == "" {
		return "", fmt.Errorf("no pudl repository found (run `pudl init`)")
	}
	return root, nil
}

func init() {
	rootCmd.AddCommand(modelCmd)
	modelCmd.AddCommand(modelPopulatorCmd)
	modelPopulatorCmd.AddCommand(modelPopulatorAddCmd)
	modelPopulatorAddCmd.Flags().BoolVar(&modelPopulatorGlobal, "global", false, "install into the global ~/.pudl repo (default: project repo if found)")
}
