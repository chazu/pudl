package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/drift"
)

// StatusOutput represents JSON output for the status command.
type StatusOutput struct {
	Definition string `json:"definition"`
	Status     string `json:"status"`
	UpdatedAt  string `json:"updated_at"`
	DiffCount  int    `json:"diff_count,omitempty"`
}

// StatusDetailOutput represents JSON output for a single definition's detailed status.
type StatusDetailOutput struct {
	Definition      string             `json:"definition"`
	Status          string             `json:"status"`
	UpdatedAt       string             `json:"updated_at"`
	DiffCount       int                `json:"diff_count,omitempty"`
	Differences     []drift.FieldDiff  `json:"differences,omitempty"`
}

var statusCmd = &cobra.Command{
	Use:   "status [definition]",
	Short: "Show convergence status of definitions",
	Long: `Display the current convergence status of all definitions or a specific one.

Status values:
  unknown     — no drift check has been run
  clean       — drift check found no differences
  drifted     — drift check found differences
  converging  — actions exported to mu, awaiting result
  converged   — mu build succeeded
  failed      — mu build failed

Examples:
    pudl status
    pudl status my_app
    pudl status --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return runStatusDetail(args[0])
		}
		return runStatusAll()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

// colorForStatus returns a lipgloss style for the given status string.
func colorForStatus(status string) lipgloss.Style {
	switch status {
	case "clean", "converged":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case "drifted", "converging":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // gray
	}
}

func runStatusAll() error {
	configDir := config.GetPudlDir()
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return fmt.Errorf("failed to open catalog: %w", err)
	}
	defer db.Close()

	statuses, err := db.GetDefinitionStatuses()
	if err != nil {
		return fmt.Errorf("failed to get definition statuses: %w", err)
	}

	if len(statuses) == 0 {
		out := GetOutputWriter()
		if jsonOutput {
			return out.WriteJSON([]StatusOutput{})
		}
		fmt.Println("No definitions found.")
		return nil
	}

	// Enrich with diff counts from drift reports
	cfg, _ := config.Load()
	var reportStore *drift.ReportStore
	if cfg != nil {
		reportStore = drift.NewReportStore(cfg.DataPath)
	}
	enrichDiffCounts(statuses, reportStore)

	out := GetOutputWriter()
	if jsonOutput {
		jsonOut := make([]StatusOutput, len(statuses))
		for i, s := range statuses {
			jsonOut[i] = StatusOutput{
				Definition: s.Definition,
				Status:     s.Status,
				UpdatedAt:  formatStatusTime(s.UpdatedAt),
				DiffCount:  s.DiffCount,
			}
		}
		return out.WriteJSON(jsonOut)
	}

	printStatusTable(statuses)
	return nil
}

func runStatusDetail(name string) error {
	configDir := config.GetPudlDir()
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return fmt.Errorf("failed to open catalog: %w", err)
	}
	defer db.Close()

	statuses, err := db.GetDefinitionStatuses()
	if err != nil {
		return fmt.Errorf("failed to get definition statuses: %w", err)
	}

	// Find the requested definition
	var found *database.DefinitionStatus
	for i := range statuses {
		if statuses[i].Definition == name {
			found = &statuses[i]
			break
		}
	}

	if found == nil {
		return fmt.Errorf("definition %q not found", name)
	}

	// Load drift report if available
	cfg, _ := config.Load()
	var latestReport *drift.DriftResult
	if cfg != nil {
		store := drift.NewReportStore(cfg.DataPath)
		latestReport, _ = store.GetLatest(name)
	}

	if latestReport != nil {
		found.DiffCount = len(latestReport.Differences)
	}

	out := GetOutputWriter()
	if jsonOutput {
		detail := StatusDetailOutput{
			Definition: found.Definition,
			Status:     found.Status,
			UpdatedAt:  formatStatusTime(found.UpdatedAt),
			DiffCount:  found.DiffCount,
		}
		if latestReport != nil && len(latestReport.Differences) > 0 {
			detail.Differences = latestReport.Differences
		}
		return out.WriteJSON(detail)
	}

	printStatusDetail(found, latestReport)
	return nil
}

func enrichDiffCounts(statuses []database.DefinitionStatus, store *drift.ReportStore) {
	if store == nil {
		return
	}
	for i := range statuses {
		if statuses[i].Status == "drifted" {
			report, err := store.GetLatest(statuses[i].Definition)
			if err == nil && report != nil {
				statuses[i].DiffCount = len(report.Differences)
			}
		}
	}
}

func printStatusTable(statuses []database.DefinitionStatus) {
	// Compute column widths
	defWidth := len("Definition")
	statusWidth := len("Status")
	for _, s := range statuses {
		if len(s.Definition) > defWidth {
			defWidth = len(s.Definition)
		}
		statusText := s.Status
		if s.DiffCount > 0 {
			statusText = fmt.Sprintf("%s (%d differences)", s.Status, s.DiffCount)
		}
		if len(statusText) > statusWidth {
			statusWidth = len(statusText)
		}
	}

	// Header
	fmt.Printf("%-*s  %-*s  %s\n", defWidth, "Definition", statusWidth, "Status", "Last Updated")
	fmt.Printf("%s  %s  %s\n",
		strings.Repeat("\u2500", defWidth),
		strings.Repeat("\u2500", statusWidth),
		strings.Repeat("\u2500", 20))

	// Rows
	for _, s := range statuses {
		styledStatus := colorForStatus(s.Status).Render(s.Status)
		extra := ""
		if s.DiffCount > 0 {
			extra = fmt.Sprintf(" (%d differences)", s.DiffCount)
		}

		ts := formatStatusTime(s.UpdatedAt)
		if ts == "" {
			ts = "\u2014"
		}

		// For padding: we need to account for ANSI codes in the styled status.
		// Plain status length determines padding needed.
		plainLen := len(s.Status) + len(extra)
		padding := ""
		if plainLen < statusWidth {
			padding = strings.Repeat(" ", statusWidth-plainLen)
		}

		fmt.Printf("%-*s  %s%s%s  %s\n", defWidth, s.Definition, styledStatus, extra, padding, ts)
	}
}

func printStatusDetail(ds *database.DefinitionStatus, report *drift.DriftResult) {
	styledStatus := colorForStatus(ds.Status).Render(ds.Status)
	ts := formatStatusTime(ds.UpdatedAt)
	if ts == "" {
		ts = "\u2014"
	}

	fmt.Printf("Definition: %s\n", ds.Definition)
	fmt.Printf("Status:     %s\n", styledStatus)
	fmt.Printf("Last Check: %s\n", ts)

	if report != nil && len(report.Differences) > 0 {
		fmt.Printf("\nDifferences (%d):\n", len(report.Differences))
		for _, d := range report.Differences {
			switch d.Type {
			case "changed":
				fmt.Printf("  ~ %s: %v -> %v\n", d.Path, d.Declared, d.Live)
			case "added":
				fmt.Printf("  + %s: %v\n", d.Path, d.Live)
			case "removed":
				fmt.Printf("  - %s: %v\n", d.Path, d.Declared)
			}
		}
	} else if ds.Status == "clean" || ds.Status == "converged" {
		fmt.Println("\nNo drift detected.")
	}
}

func formatStatusTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
