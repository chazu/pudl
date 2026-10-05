package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

var runReportCmd = &cobra.Command{
	Use:   "report [operation-id]",
	Short: "Read the latest or named standalone/set operation report",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
		if err != nil {
			return err
		}
		defer db.Close()
		return readOperationReport(db, args)
	},
}

func readOperationReport(db *database.CatalogDB, args []string) error {
	var single *database.RunReportRecord
	var set *database.RunSetReportRecord
	var err error
	if len(args) == 1 {
		single, err = db.GetRunReport(args[0])
		if err == nil {
			set, err = db.GetRunSetReport(args[0])
		}
		if single != nil && set != nil {
			return fmt.Errorf("operation id %q is ambiguous", args[0])
		}
	} else {
		single, err = db.LatestRunReport()
		if err == nil {
			set, err = db.LatestRunSetReport()
		}
	}
	if err != nil {
		return err
	}
	if single == nil && set == nil {
		if len(args) == 1 {
			return fmt.Errorf("operation report %q not found", args[0])
		}
		return fmt.Errorf("no persisted operation reports")
	}
	if set != nil && (single == nil || !single.CreatedAt.After(set.CreatedAt)) {
		if jsonOutput {
			fmt.Fprintln(outw(), string(set.Report))
			return nil
		}
		var report acute.RunSetReport
		if err := json.Unmarshal(set.Report, &report); err != nil {
			return fmt.Errorf("decode stored set report %q: %w", set.RunSetID, err)
		}
		return printRunSetReport(&report)
	}
	if jsonOutput {
		fmt.Fprintln(outw(), string(single.Report))
		return nil
	}
	var report RunReport
	if err := json.Unmarshal(single.Report, &report); err != nil {
		return fmt.Errorf("decode stored run report %q: %w", single.RunID, err)
	}
	text, err := report.render(false)
	if err != nil {
		return err
	}
	fmt.Fprint(outw(), text)
	return nil
}

func isRunSetOperation(id string) (bool, error) {
	db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
	if err != nil {
		return false, err
	}
	defer db.Close()
	single, err := db.GetRunReport(id)
	if err != nil {
		return false, err
	}
	set, err := db.GetRunSetReport(id)
	if err != nil {
		return false, err
	}
	if single != nil && set != nil {
		return false, fmt.Errorf("operation id %q is ambiguous", id)
	}
	return set != nil, nil
}

func persistRunReport(cat *runCatalog, report *RunReport, live bool) bool {
	if report == nil || report.RunID == "" {
		return false
	}
	if report.ReportVersion == 0 {
		report.ReportVersion = 1
	}
	db, err := cat.optional()
	if err != nil {
		if live {
			fmt.Fprintf(errw(), "warning: could not open catalog to persist run report: %v\n", err)
		}
		return false
	}
	b, err := json.Marshal(report)
	if err != nil {
		if live {
			fmt.Fprintf(errw(), "warning: could not encode run report: %v\n", err)
		}
		return false
	}
	if err := db.SaveRunReport(report.RunID, report.Model, b); err != nil {
		if live {
			fmt.Fprintf(errw(), "warning: could not persist run report: %v\n", err)
		}
		return false
	}
	return true
}

func init() {
	runCmd.AddCommand(runReportCmd)
}
