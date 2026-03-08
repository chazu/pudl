package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/artifact"
	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/definition"
	"pudl/internal/errors"
	"pudl/internal/executor"
	"pudl/internal/glojure"
	"pudl/internal/model"
	"pudl/internal/vault"
)

var (
	methodDryRun    bool
	methodSkipAdvice bool
	methodTags      []string
)

var methodRunCmd = &cobra.Command{
	Use:   "run <definition> <method>",
	Short: "Execute a method on a definition",
	Long: `Execute a method on a definition with full lifecycle dispatch.

Qualifications (advice) run before the action. If any fail, the action
is aborted. Post-actions (attribute/codegen methods) run after.

Flags:
  --dry-run       Run qualifications only, skip the action
  --skip-advice   Skip qualification checks
  --tag k=v       Pass extra arguments (repeatable)

Examples:
    pudl method run prod_instance list
    pudl method run prod_instance create --dry-run
    pudl method run prod_instance create --tag env=staging`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMethodRunCommand(args[0], args[1])
	},
}

func init() {
	methodCmd.AddCommand(methodRunCmd)
	methodRunCmd.Flags().BoolVar(&methodDryRun, "dry-run", false, "Run qualifications only")
	methodRunCmd.Flags().BoolVar(&methodSkipAdvice, "skip-advice", false, "Skip qualification checks")
	methodRunCmd.Flags().StringArrayVar(&methodTags, "tag", nil, "Extra args as key=value (repeatable)")
}

func runMethodRunCommand(defName, methodName string) error {
	cfg, err := config.Load()
	if err != nil {
		return errors.NewConfigError("Failed to load configuration", err)
	}

	// Initialize Glojure runtime
	rt := glojure.New()
	if err := rt.Init(); err != nil {
		return fmt.Errorf("failed to initialize Glojure runtime: %w", err)
	}

	registry := glojure.NewRegistry(rt)
	if err := glojure.RegisterBuiltins(registry); err != nil {
		return fmt.Errorf("failed to register builtin functions: %w", err)
	}

	// Create discoverers
	modelDisc := model.NewDiscoverer(cfg.SchemaPath)
	defDisc := definition.NewDiscoverer(cfg.SchemaPath)

	// Methods dir defaults to <schemaPath>/methods/
	methodsDir := cfg.SchemaPath + "/methods"

	// Create vault (non-fatal on failure)
	var v vault.Vault
	v, vaultErr := vault.New(cfg.VaultBackend, config.GetPudlDir())
	if vaultErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: vault not available: %v\n", vaultErr)
		v = nil
	}

	// Create executor
	exec := executor.New(rt, registry, modelDisc, defDisc, methodsDir, v)

	// Parse tags
	tags := make(map[string]string)
	for _, t := range methodTags {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid tag format %q, expected key=value", t)
		}
		tags[parts[0]] = parts[1]
	}

	// Run
	result, err := exec.Run(context.Background(), executor.RunOptions{
		DefinitionName: defName,
		MethodName:     methodName,
		DryRun:         methodDryRun,
		SkipAdvice:     methodSkipAdvice,
		Tags:           tags,
	})

	// Print qualification results
	if result != nil && len(result.Qualifications) > 0 {
		fmt.Println("Qualifications:")
		for _, q := range result.Qualifications {
			status := "PASS"
			if !q.Passed {
				status = "FAIL"
			}
			fmt.Printf("  [%s] %s: %s\n", status, q.Name, q.Message)
		}
		fmt.Println()
	}

	if err != nil {
		return err
	}

	// Print result
	if result.Output != nil {
		fmt.Printf("Result: %v\n", result.Output)
	}

	// Store artifact (non-fatal on failure)
	if result.Output != nil && !methodDryRun {
		storeArtifact(cfg, defName, methodName, result.Output, tags)
	}

	// Print post-action results
	if len(result.PostActions) > 0 {
		fmt.Println("\nPost-actions:")
		for _, pa := range result.PostActions {
			if pa.Error != nil {
				fmt.Printf("  [ERR]  %s: %v\n", pa.Name, pa.Error)
			} else {
				fmt.Printf("  [OK]   %s: %v\n", pa.Name, pa.Output)
			}
		}
	}

	return nil
}

func storeArtifact(cfg *config.Config, defName, methodName string, output interface{}, tags map[string]string) {
	configDir := config.GetPudlDir()
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open catalog for artifact storage: %v\n", err)
		return
	}
	defer db.Close()

	result, err := artifact.Store(db, artifact.StoreOptions{
		Definition: defName,
		Method:     methodName,
		Output:     output,
		Tags:       tags,
		DataPath:   cfg.DataPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to store artifact: %v\n", err)
		return
	}

	if result.Deduped {
		fmt.Printf("\nArtifact unchanged: %s (deduplicated)\n", result.Proquint)
	} else {
		fmt.Printf("\nArtifact stored: %s\n", result.Proquint)
	}
}
