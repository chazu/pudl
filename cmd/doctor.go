package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/doctor"
)

var doctorEntry string
var doctorHealthOnly bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check workspace health, catalog validation, and inference stability",
	Long: `Check workspace structure, integrity, schemas, and retained catalog data.
Catalog records are validated against their assigned schemas. Ordinary inferred imports
also receive a fixed-point inference check; explicit producer assignments are never
reclassified. This command does not repair or change retained data.
Use --entry ID to inspect one catalog record, or --health-only for workspace checks.`,
	Args: cobra.NoArgs,
	RunE: runDoctorCommand,
}

type healthDiagnostic struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

type doctorReport struct {
	OK      bool                `json:"ok"`
	Health  []healthDiagnostic  `json:"health"`
	Catalog []catalogDiagnostic `json:"catalog"`
	Error   string              `json:"error,omitempty"`
}

func runDoctorCommand(cmd *cobra.Command, args []string) error {
	report := doctorReport{OK: true, Health: []healthDiagnostic{}, Catalog: []catalogDiagnostic{}}
	if doctorEntry == "" {
		for _, check := range workspaceHealthChecks(effectivePudlDir()) {
			result := check.CheckFunc()
			report.Health = append(report.Health, healthDiagnostic{Name: check.Name, Status: result.Status, Message: result.Message, Details: result.Details, Fix: result.Fix})
			if result.Status == "error" {
				report.OK = false
			}
		}
	}
	if !doctorHealthOnly && report.OK {
		entries, err := checkCatalogEntries(doctorEntry)
		if err != nil {
			report.OK, report.Error = false, err.Error()
		} else {
			report.Catalog = entries
			for _, entry := range entries {
				if entry.Status != "ok" {
					report.OK = false
				}
			}
		}
	}
	if jsonOutput {
		if err := GetOutputWriter().WriteJSON(report); err != nil {
			return err
		}
	} else {
		for _, health := range report.Health {
			displayCheckResult(health.Name, &doctor.CheckResult{Status: health.Status, Message: health.Message, Details: health.Details, Fix: health.Fix})
		}
		for _, entry := range report.Catalog {
			fmt.Fprintf(outw(), "%s [%s]: %s", entry.Proquint, entry.Schema, entry.Status)
			if entry.InferredSchema != "" {
				fmt.Fprintf(outw(), " (inferred %s)", entry.InferredSchema)
			}
			if entry.Error != "" {
				fmt.Fprintf(outw(), " — %s", entry.Error)
			}
			fmt.Fprintln(outw())
		}
		if report.Error != "" {
			fmt.Fprintln(outw(), report.Error)
		}
		fmt.Fprintf(outw(), "Catalog: %d entries checked\n", len(report.Catalog))
		if report.OK {
			fmt.Fprintln(outw(), "Health check passed")
		} else {
			fmt.Fprintln(outw(), "Health check failed")
		}
	}
	if !report.OK {
		return fmt.Errorf("PUDL diagnostics failed; inspect the reported problems")
	}
	return nil
}

func workspaceHealthChecks(pudlDir string) []doctor.HealthCheck {
	return []doctor.HealthCheck{
		{
			Name:      "Workspace Structure",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckWorkspaceStructureAt(pudlDir) },
		},
		{
			Name:      "Database Integrity",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckDatabaseIntegrityAt(pudlDir) },
		},
		{
			Name:      "Schema Repository",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckSchemaRepositoryAt(pudlDir) },
		},
		{
			Name:      "Git Repository",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckGitRepositoryAt(pudlDir) },
		},
		{
			Name:      "Directory Structure",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckDirectoryStructureAt(pudlDir) },
		},
		{
			Name:      "Schema Namespace",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckPudlNamespaceSchemasAt(pudlDir) },
		},
		{Name: "Schema Loading", CheckFunc: func() *doctor.CheckResult { return doctor.CheckSchemaLoadingAt(pudlDir) }},
		{
			Name:      "Identity Fields",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckIdentityFieldConsistencyAt(pudlDir) },
		},
		{
			Name:      "Orphaned Files",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckOrphanedFilesAt(pudlDir) },
		},
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVar(&doctorEntry, "entry", "", "Check one catalog entry by proquint or full ID")
	doctorCmd.Flags().BoolVar(&doctorHealthOnly, "health-only", false, "Check workspace health without scanning catalog records")
	doctorCmd.MarkFlagsMutuallyExclusive("entry", "health-only")
	doctorCmd.RegisterFlagCompletionFunc("entry", completeProquintIDs)
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

	fmt.Fprintf(outw(), "%s %s\n", icon, name)
	fmt.Fprintf(outw(), "   Status: %s\n", result.Status)
	fmt.Fprintf(outw(), "   Message: %s\n", result.Message)

	if result.Details != "" {
		fmt.Fprintf(outw(), "   Details: %s\n", result.Details)
	}

	if result.Fix != "" {
		fmt.Fprintf(outw(), "   Fix: %s\n", result.Fix)
	}

	fmt.Fprintln(outw())
}
