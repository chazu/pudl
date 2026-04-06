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
)

var (
	driftAll  bool
	driftTags []string
)

var driftCheckCmd = &cobra.Command{
	Use:   "check [definition]",
	Short: "Check a definition for drift",
	Long: `Compare declared definition state against live state from the latest artifact.

Examples:
    pudl drift check my_instance
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
	driftCheckCmd.Flags().BoolVar(&driftAll, "all", false, "Check all definitions")
	driftCheckCmd.Flags().StringArrayVar(&driftTags, "tag", nil, "Extra args as key=value (repeatable)")
}

func runDriftCheck(name string) error {
	checker, cleanup, err := initDriftChecker()
	if err != nil {
		return err
	}
	defer cleanup()

	result, err := checker.Check(context.Background(), drift.CheckOptions{
		DefinitionName: name,
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

	for _, def := range defs {
		result, err := checker.Check(context.Background(), drift.CheckOptions{
			DefinitionName: def.Name,
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

	defDisc := definition.NewDiscoverer(cfg.SchemaPath)

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

	checker := drift.NewChecker(defDisc, db, cfg.DataPath)
	if wsCtx != nil && wsCtx.Workspace != nil {
		checker.SetOriginFilter(wsCtx.EffectiveOrigin)
	}
	return checker, cleanup, nil
}

func printDriftResult(result *drift.DriftResult) {
	fmt.Printf("Definition: %s\n", result.Definition)
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
			fmt.Printf("  ~ %s: %v -> %v\n", d.Path, d.Declared, d.Live)
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
