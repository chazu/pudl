package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

// StatusOutput represents JSON output for the status command.
type StatusOutput struct {
	Definition string `json:"definition"`
	Status     string `json:"status"`
	UpdatedAt  string `json:"updated_at"`
}

var statusCmd = &cobra.Command{
	Use:   "status [definition]",
	Short: "Show convergence status of models and definitions",
	Long: `Display the convergence status recorded in the catalog — the per-definition
status the run loop writes (a model run's verdict is recorded on its instance row,
"//models/<name>").

Status values:
  unknown     — no status recorded yet
  clean       — observed == desired (no drift)
  drifted     — observed != desired
  converging  — actions applied, pending re-verification
  converged   — drift re-check confirmed observed == desired
  failed      — a converge run failed (cap exhausted or execute error)

Examples:
    pudl status
    pudl status //models/github-chazu
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

func openCatalogForStatus() (*database.CatalogDB, error) {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return nil, fmt.Errorf("failed to open catalog: %w", err)
	}
	return db, nil
}

func runStatusAll() error {
	db, err := openCatalogForStatus()
	if err != nil {
		return err
	}
	defer db.Close()

	statuses, err := db.GetDefinitionStatuses()
	if err != nil {
		return fmt.Errorf("failed to get definition statuses: %w", err)
	}

	if len(statuses) == 0 {
		if jsonOutput {
			return GetOutputWriter().WriteJSON([]StatusOutput{})
		}
		fmt.Println("No statuses recorded.")
		return nil
	}

	if jsonOutput {
		jsonOut := make([]StatusOutput, len(statuses))
		for i, s := range statuses {
			jsonOut[i] = StatusOutput{
				Definition: s.Definition,
				Status:     s.Status,
				UpdatedAt:  formatStatusTime(s.UpdatedAt),
			}
		}
		return GetOutputWriter().WriteJSON(jsonOut)
	}

	printStatusTable(statuses)
	return nil
}

func runStatusDetail(name string) error {
	db, err := openCatalogForStatus()
	if err != nil {
		return err
	}
	defer db.Close()

	statuses, err := db.GetDefinitionStatuses()
	if err != nil {
		return fmt.Errorf("failed to get definition statuses: %w", err)
	}

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

	if jsonOutput {
		return GetOutputWriter().WriteJSON(StatusOutput{
			Definition: found.Definition,
			Status:     found.Status,
			UpdatedAt:  formatStatusTime(found.UpdatedAt),
		})
	}

	printStatusDetail(found)
	return nil
}

func printStatusTable(statuses []database.DefinitionStatus) {
	defWidth := len("Definition")
	statusWidth := len("Status")
	for _, s := range statuses {
		if len(s.Definition) > defWidth {
			defWidth = len(s.Definition)
		}
		if len(s.Status) > statusWidth {
			statusWidth = len(s.Status)
		}
	}

	fmt.Printf("%-*s  %-*s  %s\n", defWidth, "Definition", statusWidth, "Status", "Last Updated")
	fmt.Printf("%s  %s  %s\n",
		strings.Repeat("─", defWidth),
		strings.Repeat("─", statusWidth),
		strings.Repeat("─", 20))

	for _, s := range statuses {
		styledStatus := colorForStatus(s.Status).Render(s.Status)
		ts := formatStatusTime(s.UpdatedAt)
		if ts == "" {
			ts = "—"
		}
		// Pad by plain length, since the styled status carries ANSI codes.
		padding := ""
		if len(s.Status) < statusWidth {
			padding = strings.Repeat(" ", statusWidth-len(s.Status))
		}
		fmt.Printf("%-*s  %s%s  %s\n", defWidth, s.Definition, styledStatus, padding, ts)
	}
}

func printStatusDetail(ds *database.DefinitionStatus) {
	styledStatus := colorForStatus(ds.Status).Render(ds.Status)
	ts := formatStatusTime(ds.UpdatedAt)
	if ts == "" {
		ts = "—"
	}
	fmt.Printf("Definition: %s\n", ds.Definition)
	fmt.Printf("Status:     %s\n", styledStatus)
	fmt.Printf("Last Update: %s\n", ts)
}

func formatStatusTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
