package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

type approvalRequest struct {
	Model      string   `json:"model"`
	Only       []string `json:"only,omitempty"`
	MaxIters   int      `json:"max_iters"`
	MaxApplies int      `json:"max_applies"`
}

var runResumeCmd = &cobra.Command{
	Use:     "resume <run-id>",
	Short:   "Approve and continue a pending converge run",
	Aliases: []string{"approve"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.NewCatalogDB(config.GetPudlDir())
		if err != nil {
			return err
		}
		approval, err := db.GetRunApproval(args[0])
		if err != nil {
			db.Close()
			return err
		}
		if approval == nil || approval.Status != "pending" {
			db.Close()
			return fmt.Errorf("run %q has no pending approval", args[0])
		}
		var request approvalRequest
		if err := json.Unmarshal(approval.Request, &request); err != nil {
			db.Close()
			return fmt.Errorf("decode approval request %q: %w", args[0], err)
		}
		if err := db.ResolveRunApproval(args[0], "approved"); err != nil {
			db.Close()
			return err
		}
		db.Close()

		// Re-enter the same execution path with the original run identity. The
		// pending run row is intentionally unfinished until this invocation ends.
		runConverge = true
		runRequireApproval = false
		runResumeID = args[0]
		runApprovalStatus = "approved"
		runOnly = request.Only
		runMaxIters = request.MaxIters
		runMaxApplies = request.MaxApplies
		return runCmd.RunE(runCmd, []string{request.Model})
	},
}

var runRejectCmd = &cobra.Command{
	Use:   "reject <run-id>",
	Short: "Reject a pending converge run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := database.NewCatalogDB(config.GetPudlDir())
		if err != nil {
			return err
		}
		defer db.Close()
		approval, err := db.GetRunApproval(args[0])
		if err != nil {
			return err
		}
		if approval == nil || approval.Status != "pending" {
			return fmt.Errorf("run %q has no pending approval", args[0])
		}
		if err := db.ResolveRunApproval(args[0], "rejected"); err != nil {
			return err
		}
		if err := db.FinishRun(args[0], database.RunConclusion{Verdict: "failed", Outcome: "rejected", Note: "convergence approval rejected"}); err != nil {
			return err
		}
		report, _ := json.Marshal(&RunReport{
			RunID: args[0], Model: approval.Model, Mode: "converge", OK: false,
			Error: "convergence approval rejected", ApprovalStatus: "rejected",
		})
		if err := db.SaveRunReport(args[0], approval.Model, report); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Println(string(report))
		} else {
			fmt.Printf("rejected converge run %s\n", args[0])
		}
		return nil
	},
}

func init() {
	runCmd.AddCommand(runResumeCmd, runRejectCmd)
}
