package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/definition"
	"pudl/internal/executor"
	"pudl/internal/glojure"
	"pudl/internal/model"
	"pudl/internal/vault"
	"pudl/internal/workflow"
)

var (
	wfDryRun         bool
	wfTags           []string
	wfMaxConcurrency int
)

var workflowRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Execute a workflow",
	Long: `Execute a multi-step workflow with concurrent step dispatch.

Examples:
    pudl workflow run file_pipeline
    pudl workflow run file_pipeline --dry-run
    pudl workflow run file_pipeline --tag env=staging
    pudl workflow run file_pipeline --max-concurrency 2`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWorkflowRunCommand(args[0])
	},
}

func init() {
	workflowCmd.AddCommand(workflowRunCmd)
	workflowRunCmd.Flags().BoolVar(&wfDryRun, "dry-run", false, "Run qualifications only")
	workflowRunCmd.Flags().StringArrayVar(&wfTags, "tag", nil, "Extra args as key=value (repeatable)")
	workflowRunCmd.Flags().IntVar(&wfMaxConcurrency, "max-concurrency", 0, "Max concurrent steps (0=unlimited)")
}

func runWorkflowRunCommand(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Discover workflow
	disc := workflow.NewDiscoverer(cfg.SchemaPath)
	wf, err := disc.GetWorkflow(name)
	if err != nil {
		return err
	}

	// Initialize runtime
	rt := glojure.New()
	if err := rt.Init(); err != nil {
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}

	registry := glojure.NewRegistry(rt)
	if err := glojure.RegisterBuiltins(registry); err != nil {
		return fmt.Errorf("failed to register builtins: %w", err)
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

	// Open catalog DB
	configDir := config.GetPudlDir()
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: catalog not available: %v\n", err)
	}
	if db != nil {
		defer db.Close()
	}

	// Parse tags
	tags := make(map[string]string)
	for _, t := range wfTags {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid tag format %q, expected key=value", t)
		}
		tags[parts[0]] = parts[1]
	}

	// Run workflow
	runner := workflow.NewRunner(exec, db, cfg.DataPath)
	result, err := runner.Run(context.Background(), wf, workflow.RunOptions{
		DryRun:         wfDryRun,
		Tags:           tags,
		MaxConcurrency: wfMaxConcurrency,
	})
	if err != nil {
		return err
	}

	// Print results
	fmt.Printf("Workflow: %s\n", result.WorkflowName)
	fmt.Printf("Run ID:  %s\n", result.RunID)
	fmt.Printf("Status:  %s\n", result.Status)
	fmt.Printf("Duration: %s\n\n", result.Duration.Round(time.Millisecond))

	fmt.Println("Steps:")
	for name, sr := range result.Steps {
		fmt.Printf("  %s: %s", name, sr.Status)
		if sr.ArtifactID != "" {
			fmt.Printf(" (artifact: %s)", sr.ArtifactID)
		}
		if sr.Error != "" {
			fmt.Printf(" - %s", sr.Error)
		}
		fmt.Println()
	}

	if result.Status == "failed" {
		return fmt.Errorf("workflow %q failed", name)
	}

	return nil
}
