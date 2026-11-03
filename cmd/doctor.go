package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/doctor"
	"github.com/chazu/pudl/internal/repair"
)

var doctorEntry string
var doctorHealthOnly bool
var doctorVerifyPayloads bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check workspace health and catalog data, and repair the catalog",
	Long: `Check workspace structure, integrity, schemas, and retained catalog data.
Catalog records are validated against their assigned schemas. Ordinary inferred imports
also receive a fixed-point inference check; explicit producer assignments are never
reclassified. Use --entry ID to inspect one catalog record, or --health-only for workspace checks.
The catalog scan also warns of likely secrets in stored payloads and current facts,
naming the entry or fact, path and kind, never the value.

Doctor also diagnoses catalog conditions that a repair fixes, and names the repair:
  schema-names  schema names not in canonical <package>.#<Definition> form

  policy-leaves entries assigned to a policy schema instead of its type
  inference     inferred entries whose schema inference now selects another schema
  identity      entries missing a resource_id, or namespaced by a leaf schema
  projections   projected facts that no longer match the catalog or facts specs
  policy-results policy results that are stale (evaluated with an older policy
                digest), orphaned, or never evaluated
  data-permissions  data files or directories group or other users can access
  sensitive-scrub   plaintext stored before a schema declared the field in
                _pudl.sensitive_fields (opt-in: applied only when --only names it)

Without --fix nothing is written. --fix applies every repair (or those named by
--only) in that order; --fix --dry-run previews them. The sensitive scrub
re-derives the IDs of the entries it rewrites (old IDs still resolve) and
compacts the catalog; fact history and exported bundles keep the plaintext, so
rotate every credential it redacts.`,
	Example: `  pudl doctor
  pudl doctor --fix --dry-run
  pudl doctor --fix --only projections
  pudl doctor --fix --dry-run --only sensitive-scrub
  pudl doctor --fix --only inference --schema pudl/core.#Item --include-explicit`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runDoctorCommand,
}

type healthDiagnostic struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

// doctorReportVersion 2 replaced argv-only repair suggestions with repair
// results; 3 added secrets. Reports without report_version are version 1.
const doctorReportVersion = 3

type doctorReport struct {
	ReportVersion int                        `json:"report_version"`
	Mode          string                     `json:"mode"`
	Repairs       []doctorRepair             `json:"repairs"`
	OK            bool                       `json:"ok"`
	Health        []healthDiagnostic         `json:"health"`
	Catalog       []doctor.CatalogDiagnostic `json:"catalog"`
	// Secrets is present when stored data was scanned. Findings warn: they
	// never fail doctor, because detection is heuristic.
	Secrets *doctor.SecretReport `json:"secrets,omitempty"`
	Error   string               `json:"error,omitempty"`
}

func runDoctorCommand(cmd *cobra.Command, args []string) error {
	if doctorDryRun && !doctorFix {
		return fmt.Errorf("--dry-run previews repairs and requires --fix")
	}
	selected, err := repair.Select(doctorOnly)
	if err != nil {
		return err
	}
	report := doctorReport{ReportVersion: doctorReportVersion, Mode: doctorMode(), OK: true, Health: []healthDiagnostic{}, Catalog: []doctor.CatalogDiagnostic{}}
	if doctorEntry == "" {
		for _, check := range workspaceHealthChecks(effectivePudlDir()) {
			result := check.CheckFunc()
			report.Health = append(report.Health, healthDiagnostic{Name: check.Name, Status: result.Status, Message: result.Message, Details: result.Details, Fix: result.Fix})
			if result.Status == "error" {
				report.OK = false
			}
		}
	}
	if doctorVerifyPayloads {
		result := doctor.CheckPayloadsAt(cmd.Context(), effectivePudlDir())
		report.Health = append(report.Health, healthDiagnostic{Name: "Payload Hashes", Status: result.Status, Message: result.Message, Details: result.Details, Fix: result.Fix})
		if result.Status == "error" {
			report.OK = false
		}
	}
	healthOK := report.OK
	scanCatalog := func() {
		if doctorHealthOnly || !healthOK {
			return
		}
		entries, secrets, err := checkCatalogEntries(cmd.Context())
		report.OK, report.Error, report.Catalog, report.Secrets = healthOK, "", []doctor.CatalogDiagnostic{}, nil
		if err != nil {
			report.OK, report.Error = false, err.Error()
			return
		}
		report.Catalog, report.Secrets = entries, secrets
		for _, entry := range entries {
			if entry.Status != "ok" {
				report.OK = false
			}
		}
	}
	scanCatalog()
	scanned := !doctorHealthOnly && healthOK && report.Error == ""
	report.Repairs = runDoctorRepairs(cmd.Context(), selected, healthOK, report.Catalog, scanned)
	if doctorFix && !doctorDryRun && repairsChanged(report.Repairs) {
		scanCatalog() // report the catalog as the repairs left it
	}
	if doctorFix && repairsFailed(report.Repairs) {
		report.OK = false
	}
	if jsonOutput {
		if err := GetOutputWriter().WriteJSON(report); err != nil {
			return err
		}
	} else {
		printDoctorReport(report)
	}
	if !report.OK {
		return fmt.Errorf("PUDL diagnostics failed; inspect the reported problems")
	}
	return nil
}

// checkCatalogEntries validates the catalog entries --entry, --schema and
// --origin select against the workspace's schemas.
func checkCatalogEntries(ctx context.Context) ([]doctor.CatalogDiagnostic, *doctor.SecretReport, error) {
	cfg, err := loadEffectiveConfig()
	if err != nil {
		return nil, nil, err
	}
	return doctor.CheckCatalogEntries(ctx, doctor.CatalogScan{
		DataPath: cfg.DataPath, SchemaPaths: effectiveSchemaPaths(cfg), EntryID: doctorEntry,
		Filter: database.FilterOptions{Schema: doctorSchema, Origin: doctorOrigin},
	})
}

func printDoctorReport(report doctorReport) {
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
	printDoctorSecrets(report.Secrets)
	printDoctorRepairs(report.Repairs)
	fmt.Fprintf(outw(), "Catalog: %d entries checked\n", len(report.Catalog))
	if report.OK {
		fmt.Fprintln(outw(), "Health check passed")
	} else {
		fmt.Fprintln(outw(), "Health check failed")
	}
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
			Name:      "Resource Versions",
			CheckFunc: func() *doctor.CheckResult { return doctor.CheckResourceVersionsAt(pudlDir) },
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
		{Name: "Datalog Rules", CheckFunc: func() *doctor.CheckResult { return doctor.CheckRulesAt(queryRulePaths(pudlDir)...) }},
		{Name: "Graph Vocabulary", CheckFunc: func() *doctor.CheckResult { return doctor.CheckGraphVocabulary(workspaceVocabulary(pudlDir)) }},
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVar(&doctorVerifyPayloads, "verify-payloads", false, "Verify referenced raw payload hashes without repairing evidence")
	doctorCmd.Flags().StringVar(&doctorEntry, "entry", "", "Check one catalog entry by proquint or full ID")
	doctorCmd.Flags().BoolVar(&doctorHealthOnly, "health-only", false, "Check workspace health without scanning catalog records")
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Apply the diagnosed repairs")
	doctorCmd.Flags().BoolVar(&doctorDryRun, "dry-run", false, "With --fix, report what the repairs would change without writing")
	doctorCmd.Flags().StringSliceVar(&doctorOnly, "only", nil, "Limit repairs to these names: "+strings.Join(repair.Names(), ", "))
	doctorCmd.Flags().StringVar(&doctorSchema, "schema", "", "Check and repair only entries assigned to this schema")
	doctorCmd.Flags().StringVar(&doctorOrigin, "origin", "", "Check and repair only entries from this origin")
	doctorCmd.Flags().BoolVar(&doctorIncludeExplicit, "include-explicit", false, "Re-infer every entry in scope, including explicit assignments and invalid entries")

	doctorCmd.MarkFlagsMutuallyExclusive("entry", "health-only")
	doctorCmd.MarkFlagsMutuallyExclusive("entry", "schema")
	doctorCmd.MarkFlagsMutuallyExclusive("entry", "origin")
	doctorCmd.RegisterFlagCompletionFunc("entry", completeProquintIDs)
	doctorCmd.RegisterFlagCompletionFunc("schema", completeSchemaNames)
	doctorCmd.RegisterFlagCompletionFunc("origin", completeOrigins)

	doctorCmd.RegisterFlagCompletionFunc("only", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return repair.Names(), cobra.ShellCompDirectiveNoFileComp
	})
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
