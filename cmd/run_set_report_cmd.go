package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

var runSetReportCmd = &cobra.Command{
	Use:   "report [run-set-id]",
	Short: "Read a persisted run-set report",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.NewCatalogDB(config.GetPudlDir())
		if err != nil {
			return err
		}
		defer db.Close()

		var record *database.RunSetReportRecord
		if len(args) == 1 {
			record, err = db.GetRunSetReport(args[0])
		} else {
			record, err = db.LatestRunSetReport()
		}
		if err != nil {
			return err
		}
		if record == nil {
			if len(args) == 1 {
				return fmt.Errorf("run-set report %q not found", args[0])
			}
			return fmt.Errorf("no persisted run-set reports")
		}
		if jsonOutput {
			fmt.Println(string(record.Report))
			return nil
		}

		var report acute.RunSetReport
		if err := json.Unmarshal(record.Report, &report); err != nil {
			return fmt.Errorf("decode stored run-set report %q: %w", record.RunSetID, err)
		}
		return printRunSetReport(&report)
	},
}

func init() {
	runSetCmd.AddCommand(runSetReportCmd)
}
