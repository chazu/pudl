package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

var runReportCmd = &cobra.Command{
	Use:   "report [run-id]",
	Short: "Read a persisted run report",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.NewCatalogDB(config.GetPudlDir())
		if err != nil {
			return err
		}
		defer db.Close()
		var report *database.RunReportRecord
		if len(args) == 1 {
			report, err = db.GetRunReport(args[0])
		} else {
			report, err = db.LatestRunReport()
		}
		if err != nil {
			return err
		}
		if report == nil {
			if len(args) == 1 {
				return fmt.Errorf("run report %q not found", args[0])
			}
			return fmt.Errorf("no persisted run reports")
		}
		if jsonOutput {
			fmt.Println(string(report.Report))
			return nil
		}
		var decoded RunReport
		if err := json.Unmarshal(report.Report, &decoded); err != nil {
			return fmt.Errorf("decode stored run report %q: %w", report.RunID, err)
		}
		text, err := decoded.render(false)
		if err != nil {
			return err
		}
		fmt.Print(text)
		return nil
	},
}

func persistRunReport(cat *runCatalog, report *RunReport, live bool) bool {
	if report == nil || report.RunID == "" {
		return false
	}
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Printf("warning: could not open catalog to persist run report: %v\n", err)
		}
		return false
	}
	b, err := json.Marshal(report)
	if err != nil {
		if live {
			fmt.Printf("warning: could not encode run report: %v\n", err)
		}
		return false
	}
	if err := db.SaveRunReport(report.RunID, report.Model, b); err != nil {
		if live {
			fmt.Printf("warning: could not persist run report: %v\n", err)
		}
		return false
	}
	return true
}

func init() {
	runCmd.AddCommand(runReportCmd)
}
