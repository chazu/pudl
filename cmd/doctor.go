package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pudl/internal/doctor"
	"pudl/internal/errors"
)

// doctorCmd represents the doctor command
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check PUDL workspace health",
	Long: `Run health checks on your PUDL workspace.

This command performs a series of checks to ensure your PUDL workspace
is properly configured and healthy. It checks:

- Workspace structure (required directories)
- Database integrity
- Schema repository setup
- Git repository initialization
- Orphaned files

Use this command to diagnose issues with your PUDL installation.`,
	Run: func(cmd *cobra.Command, args []string) {
		errorHandler := errors.NewCLIErrorHandler(true)

		if err := runDoctorCommand(cmd, args); err != nil {
			errorHandler.HandleError(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctorCommand(cmd *cobra.Command, args []string) error {
	// Define all health checks
	checks := []doctor.HealthCheck{
		{
			Name:      "Workspace Structure",
			CheckFunc: doctor.CheckWorkspaceStructure,
		},
		{
			Name:      "Database Integrity",
			CheckFunc: doctor.CheckDatabaseIntegrity,
		},
		{
			Name:      "Schema Repository",
			CheckFunc: doctor.CheckSchemaRepository,
		},
		{
			Name:      "Git Repository",
			CheckFunc: doctor.CheckGitRepository,
		},
		{
			Name:      "Orphaned Files",
			CheckFunc: doctor.CheckOrphanedFiles,
		},
	}

	// Run all checks and collect results
	fmt.Println("🏥 PUDL Health Check")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	var hasErrors bool
	var hasWarnings bool
	var results []struct {
		name   string
		result *doctor.CheckResult
	}

	for _, check := range checks {
		result := check.CheckFunc()
		results = append(results, struct {
			name   string
			result *doctor.CheckResult
		}{check.Name, result})

		// Track if we have errors or warnings
		if result.Status == "error" {
			hasErrors = true
		} else if result.Status == "warning" {
			hasWarnings = true
		}
	}

	// Display results
	for _, r := range results {
		displayCheckResult(r.name, r.result)
	}

	// Summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")

	if hasErrors {
		fmt.Println("❌ Health check failed - errors detected")
		return errors.NewSystemError("PUDL workspace has errors", nil)
	}

	if hasWarnings {
		fmt.Println("⚠️  Health check passed with warnings")
		return nil
	}

	fmt.Println("✅ Health check passed - workspace is healthy")
	return nil
}

func displayCheckResult(name string, result *doctor.CheckResult) {
	var icon string
	switch result.Status {
	case "ok":
		icon = "✅"
	case "warning":
		icon = "⚠️ "
	case "error":
		icon = "❌"
	default:
		icon = "❓"
	}

	fmt.Printf("%s %s\n", icon, name)
	fmt.Printf("   Status: %s\n", result.Status)
	fmt.Printf("   Message: %s\n", result.Message)

	if result.Details != "" {
		fmt.Printf("   Details: %s\n", result.Details)
	}

	if result.Fix != "" {
		fmt.Printf("   Fix: %s\n", result.Fix)
	}

	fmt.Println()
}

