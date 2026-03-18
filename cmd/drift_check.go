package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/definition"
	"pudl/internal/drift"
	"pudl/internal/executor"
	"pudl/internal/glojure"
	"pudl/internal/model"
	"pudl/internal/vault"
)

var (
	driftMethod  string
	driftRefresh bool
	driftAll     bool
	driftTags    []string
)

var driftCheckCmd = &cobra.Command{
	Use:   "check [definition]",
	Short: "Check a definition for drift",
	Long: `Compare declared definition state against live state from the latest artifact.

Examples:
    pudl drift check my_instance
    pudl drift check my_instance --method list
    pudl drift check my_instance --refresh
    pudl drift check --all
    pudl drift check my_instance --tag env=prod`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if driftAll {
			return runDriftCheckAll()
		}
		if len(args) == 0 {
			return fmt.Errorf("definition name required (or use --all)")
		}
		return runDriftCheck(args[0])
	},
}

func init() {
	driftCmd.AddCommand(driftCheckCmd)
	driftCheckCmd.Flags().StringVar(&driftMethod, "method", "", "Method whose artifact to compare (default: auto-detect)")
	driftCheckCmd.Flags().BoolVar(&driftRefresh, "refresh", false, "Re-execute the method before comparing")
	driftCheckCmd.Flags().BoolVar(&driftAll, "all", false, "Check all definitions")
	driftCheckCmd.Flags().StringArrayVar(&driftTags, "tag", nil, "Extra args as key=value (repeatable)")
}

func runDriftCheck(name string) error {
	checker, cleanup, err := initDriftChecker()
	if err != nil {
		return err
	}
	defer cleanup()

	tags := parseDriftTags()

	result, err := checker.Check(context.Background(), drift.CheckOptions{
		DefinitionName: name,
		Method:         driftMethod,
		Refresh:        driftRefresh,
		Tags:           tags,
	})
	if err != nil {
		return err
	}

	printDriftResult(result)
	return nil
}

func runDriftCheckAll() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	defDisc := definition.NewDiscoverer(cfg.SchemaPath)
	defs, err := defDisc.ListDefinitions()
	if err != nil {
		return err
	}

	if len(defs) == 0 {
		fmt.Println("No definitions found.")
		return nil
	}

	checker, cleanup, err := initDriftChecker()
	if err != nil {
		return err
	}
	defer cleanup()

	tags := parseDriftTags()

	for _, def := range defs {
		result, err := checker.Check(context.Background(), drift.CheckOptions{
			DefinitionName: def.Name,
			Method:         driftMethod,
			Refresh:        driftRefresh,
			Tags:           tags,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", def.Name, err)
			continue
		}

		printDriftResultSummary(result)
	}

	return nil
}

func initDriftChecker() (*drift.Checker, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}

	rt := glojure.New()
	if err := rt.Init(); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize runtime: %w", err)
	}

	registry := glojure.NewRegistry(rt)
	if err := glojure.RegisterBuiltins(registry); err != nil {
		return nil, nil, fmt.Errorf("failed to register builtins: %w", err)
	}

	modelDisc := model.NewDiscoverer(cfg.SchemaPath)
	defDisc := definition.NewDiscoverer(cfg.SchemaPath)
	methodsDir := cfg.SchemaPath + "/methods"

	var v vault.Vault
	v, vaultErr := vault.New(cfg.VaultBackend, config.GetPudlDir())
	if vaultErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: vault not available: %v\n", vaultErr)
		v = nil
	}

	exec := executor.New(rt, registry, modelDisc, defDisc, methodsDir, v)

	configDir := config.GetPudlDir()
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open catalog: %w", err)
	}

	cleanup := func() {
		if db != nil {
			db.Close()
		}
	}

	checker := drift.NewChecker(defDisc, modelDisc, db, exec, cfg.DataPath)
	return checker, cleanup, nil
}

func printDriftResult(result *drift.DriftResult) {
	fmt.Printf("Definition: %s\n", result.Definition)
	fmt.Printf("Method:     %s\n", result.Method)
	fmt.Printf("Status:     %s\n", result.Status)
	fmt.Printf("Checked:    %s\n", drift.FormatTimestamp(result.Timestamp))

	if len(result.Differences) == 0 {
		fmt.Println("\nNo drift detected.")
		return
	}

	fmt.Printf("\nDifferences (%d):\n", len(result.Differences))
	for _, d := range result.Differences {
		switch d.Type {
		case "changed":
			fmt.Printf("  ~ %s: %v → %v\n", d.Path, d.Declared, d.Live)
		case "added":
			fmt.Printf("  + %s: %v\n", d.Path, d.Live)
		case "removed":
			fmt.Printf("  - %s: %v\n", d.Path, d.Declared)
		}
	}
}

func printDriftResultSummary(result *drift.DriftResult) {
	status := result.Status
	diffCount := len(result.Differences)
	if diffCount > 0 {
		fmt.Printf("  %-30s %s (%d differences)\n", result.Definition, status, diffCount)
	} else {
		fmt.Printf("  %-30s %s\n", result.Definition, status)
	}
}

func parseDriftTags() map[string]string {
	tags := make(map[string]string)
	for _, t := range driftTags {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) == 2 {
			tags[parts[0]] = parts[1]
		}
	}
	return tags
}
