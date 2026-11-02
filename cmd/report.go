package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
)

var reportCmd = &cobra.Command{
	Use:   "report [ID]",
	Short: "Read the latest or named check report",
	Long: `Read a stored report: the newest one, or the one ID names.

ID is a check ID, or the run ID of one model within a check. Reports recorded
by the retired "pudl run" and "pudl run set" commands read the same way;
fields those reports carry that current reports do not are shown as recorded
under "legacy fields".

Examples:
    pudl report
    pudl report check_... --json`,
	Args: cobra.MaximumNArgs(1),
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
	var stored *checkreport.Stored
	var err error
	if len(args) == 1 {
		stored, err = checkreport.Find(db, args[0])
	} else {
		stored, err = checkreport.Latest(db)
	}
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
}
