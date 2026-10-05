package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
)

type approvalRequest struct {
	Model      string   `json:"model"`
	Only       []string `json:"only,omitempty"`
	MaxIters   int      `json:"max_iters"`
	MaxApplies int      `json:"max_applies"`
	MuRoot     string   `json:"mu_root,omitempty"`
}

func newApprovalRequest(model string, flags runFlags, muRoot string) approvalRequest {
	return approvalRequest{
		Model: model, Only: flags.only, MaxIters: flags.maxIters,
		MaxApplies: flags.maxApplies, MuRoot: muRoot,
	}
}

var runResumeCmd = &cobra.Command{
	Use:     "resume <operation-id>",
	Short:   "Approve and continue a pending run or exact set plan",
	Aliases: []string{"approve"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return resumeOperation(args[0], defaultRunDeps())
	},
}

// resumeOperation approves a pending standalone run or exact set plan and
// continues it.
func resumeOperation(operationID string, deps runDeps) error {
	isSet, err := isRunSetOperation(operationID)
	if err != nil {
		return err
	}
	if isSet {
		return resumeRunSet(operationID, deps)
	}
	return resumeRun(operationID, deps)
}

// resumeRun approves a pending standalone converge run and re-enters the run
// path with the stored request, under the pending run's identity.
func resumeRun(runID string, deps runDeps) error {
	request, err := approvePendingRun(runID)
	if err != nil {
		return err
	}
	// Re-enter the same execution path with the original run identity. The
	// pending run row is intentionally unfinished until this invocation ends.
	_, err = executeRun(resumedRunOptions(runID, request), deps)
	return err
}

// approvePendingRun marks a pending standalone approval approved and returns
// the converge request it stored. The catalog is closed before the run
// re-opens its own handle.
func approvePendingRun(runID string) (approvalRequest, error) {
	var request approvalRequest
	db, err := database.NewCatalogDB(effectivePudlDir())
	if err != nil {
		return request, err
	}
	defer db.Close()
	approval, err := db.GetRunApproval(runID)
	if err != nil {
		return request, err
	}
	if approval == nil || approval.Status != "pending" {
		return request, fmt.Errorf("run %q has no pending approval", runID)
	}
	if err := json.Unmarshal(approval.Request, &request); err != nil {
		return request, fmt.Errorf("decode approval request %q: %w", runID, err)
	}
	if err := db.ResolveRunApproval(runID, "approved"); err != nil {
		return request, err
	}
	return request, nil
}

var runRejectCmd = &cobra.Command{
	Use:   "reject <operation-id>",
	Short: "Reject a pending run or exact set plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		isSet, err := isRunSetOperation(args[0])
		if err != nil {
			return err
		}
		if isSet {
			return rejectRunSet(args[0])
		}
		db, err := database.NewCatalogDB(effectivePudlDir())
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
		if err := db.FinishRun(args[0], database.RunConclusion{CompletionStatus: database.RunStatusFailed, Verdict: "failed", Outcome: "rejected", Note: "convergence approval rejected"}); err != nil {
			return err
		}
		report, _ := json.Marshal(&RunReport{
			ReportVersion: 1, RunID: args[0], Model: approval.Model, Mode: "converge", OK: false,
			Error: "convergence approval rejected", ApprovalStatus: "rejected",
		})
		if err := db.SaveRunReport(args[0], approval.Model, report); err != nil {
			return err
		}
		if jsonOutput {
			fmt.Fprintln(outw(), string(report))
		} else {
			fmt.Fprintf(outw(), "rejected converge run %s\n", args[0])
		}
		return nil
	},
}

func init() {
	runCmd.AddCommand(runResumeCmd, runRejectCmd)
}
