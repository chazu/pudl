package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	pudlInit "github.com/chazu/pudl/internal/init"
	"github.com/chazu/pudl/internal/repo"
)

var initForce, initGlobal bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize or repair a local PUDL workspace (--global for ~/.pudl)",
	Long: `Create a self-contained .pudl/ in the current directory, including
configuration, schemas, local data directories, and bundled agent skills.
Repeating initialization repairs owned files while preserving authored configuration.
Use --force to replace configuration, or --global to initialize ~/.pudl instead.`,
	Args: cobra.NoArgs,
	RunE: runInitCommand,
}

func runInitCommand(cmd *cobra.Command, args []string) error {
	root := config.GetPudlDir()
	mode := "global"
	if initGlobal {
		if initForce || !config.Exists() {
			if err := pudlInit.Initialize(pudlInit.InitOptions{Force: initForce, Verbose: !jsonOutput}); err != nil {
				return err
			}
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		root, mode = filepath.Join(cwd, ".pudl"), "workspace"
		if err := repo.Init(repo.InitOptions{Dir: cwd, Force: initForce, Verbose: !jsonOutput}); err != nil {
			return err
		}
	}
	if jsonOutput {
		return GetOutputWriter().WriteJSON(map[string]string{"path": root, "mode": mode})
	}
	fmt.Printf("PUDL workspace ready: %s\n", root)
	fmt.Println("Next: pudl model list, or pudl import --path <file>")
	return nil
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&initForce, "force", false, "Replace authored workspace configuration")
	initCmd.Flags().BoolVar(&initGlobal, "global", false, "Initialize ~/.pudl instead of a local .pudl")
}
