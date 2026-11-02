package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
)

// reportSince is `pudl report --since`: the earlier report to compare with.
var reportSince string

var reportCmd = &cobra.Command{
	Use:   "report [ID]",
	Short: "Read the latest or named check report",
	Long: `Read a stored report: the newest one, or the one ID names.

ID is a check ID, or the run ID of one model within a check. Reports recorded
by the retired "pudl run" and "pudl run set" commands read the same way;
fields those reports carry that current reports do not are shown as recorded
under "legacy fields".

With --since ID2 it compares instead: each check's failing matches in the
report against the same check in report ID2, model by model, as new,
persisting, resolved, or unknown when the report cannot establish their
absence.

Examples:
    pudl report
    pudl report check_... --json
    pudl report check_B --since check_A`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.OpenCatalogDBReadOnly(effectivePudlDir())
		if err != nil {
			return err
		}
		defer db.Close()
		if reportSince != "" {
			return compareOperationReports(db, args, reportSince)
		}
		return readOperationReport(db, args)
	},
}

// findOperationReport reads the report args name, or the newest.
func findOperationReport(db *database.CatalogDB, args []string) (*checkreport.Stored, error) {
	if len(args) == 1 {
		return checkreport.Find(db, args[0])
	}
	return checkreport.Latest(db)
}

func readOperationReport(db *database.CatalogDB, args []string) error {
	stored, err := findOperationReport(db, args)
	if err != nil {
		return err
	}
	if jsonOutput {
		payload, err := stored.JSON()
		if err != nil {
			return err
		}
		fmt.Fprintln(outw(), string(payload))
		return nil
	}
	return stored.WriteText(outw())
}

// compareOperationReports writes the changes in the report args name (or
// the newest) since the report since names.
func compareOperationReports(db *database.CatalogDB, args []string, since string) error {
	current, err := findOperationReport(db, args)
	if err != nil {
		return err
	}
	earlier, err := checkreport.Find(db, since)
	if err != nil {
		return err
	}
	comparison, err := checkreport.Compare(db, current, earlier)
	if err != nil {
		return err
	}
	if jsonOutput {
		payload, err := json.MarshalIndent(comparison, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(outw(), string(payload))
		return err
	}
	comparison.WriteText(outw())
	return nil
}

// printCheckReport writes a check report: under --json the report itself;
// otherwise the outcome of each model and, with members, each model's full
// report first (a live check has already printed those as they concluded).
func printCheckReport(report *acute.RunSetReport, members bool) error {
	if jsonOutput {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(outw(), string(payload))
		return err
	}
	return checkreport.WriteCheck(outw(), report, members)
}

func init() {
	rootCmd.AddCommand(reportCmd)
	reportCmd.Flags().StringVar(&reportSince, "since", "", "compare the report's check findings with an earlier report (check ID or run ID): new, persisting, resolved, unknown")
}
