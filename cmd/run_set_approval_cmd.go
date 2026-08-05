package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
)

var runSetResumeCmd = &cobra.Command{
	Use:     "resume <run-set-id>",
	Short:   "Revalidate, approve, and execute a pending exact run-set plan",
	Aliases: []string{"approve"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return resumeRunSet(args[0])
	},
}

var runSetRejectCmd = &cobra.Command{
	Use:   "reject <run-set-id>",
	Short: "Reject a pending mutating run-set plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return rejectRunSet(args[0])
	},
}

func resumeRunSet(runSetID string) error {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return err
	}
	defer db.Close()
	approval, report, storedPlan, request, err := loadPendingRunSetApproval(db, runSetID)
	if err != nil {
		return err
	}
	graph, context, rebuildErr := reconstructApprovedRunSet(db, report, storedPlan, request)
	if rebuildErr != nil {
		return markRunSetApprovalStale(db, approval, report, fmt.Errorf("rebuild exact plan: %w", rebuildErr))
	}
	rebuilt, prepared, rebuildErr := buildRunSetMutationPlan(db, graph, report, context, request, true)
	if rebuildErr != nil {
		return markRunSetApprovalStale(db, approval, report, fmt.Errorf("revalidate exact plan: %w", rebuildErr))
	}
	digest, err := rebuilt.CanonicalDigest()
	if err != nil {
		return markRunSetApprovalStale(db, approval, report, err)
	}
	if digest != approval.PlanDigest {
		return markRunSetApprovalStale(db, approval, report,
			fmt.Errorf("exact plan changed: approved %s, rebuilt %s", approval.PlanDigest, digest))
	}
	report.Status = database.RunStatusRunning
	report.ApprovalStatus = "approved"
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if err := db.ApproveRunSetPlan(runSetID, approval.PlanDigest, reportJSON); err != nil {
		return err
	}
	defer func() { _ = retainMutationPlanSnapshots(db, prepared, false) }()
	return executePreparedMutationPlan(db, report, rebuilt, prepared)
}

func rejectRunSet(runSetID string) error {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return err
	}
	defer db.Close()
	approval, report, _, _, err := loadPendingRunSetApproval(db, runSetID)
	if err != nil {
		return err
	}
	if err := db.ResolveRunSetApproval(runSetID, approval.PlanDigest, "rejected"); err != nil {
		return err
	}
	note := "exact run-set approval rejected"
	report.ApprovalStatus = "rejected"
	if err := cancelPendingRunSetMembers(db, report, note); err != nil {
		return err
	}
	if err := retainReportSnapshots(db, report, false); err != nil {
		return err
	}
	report.Status = database.RunStatusFailed
	if err := saveRunSetReport(db, report); err != nil {
		return err
	}
	return printRunSetReport(report)
}

func loadPendingRunSetApproval(db *database.CatalogDB, runSetID string) (*database.RunSetApprovalRecord, *acute.RunSetReport, *acute.RunSetMutationPlan, runSetMutationRequest, error) {
	approval, err := db.GetRunSetApproval(runSetID)
	if err != nil {
		return nil, nil, nil, runSetMutationRequest{}, err
	}
	if approval == nil || approval.Status != "pending" {
		return nil, nil, nil, runSetMutationRequest{}, fmt.Errorf("run set %q has no pending exact-plan approval", runSetID)
	}
	record, err := db.GetRunSetReport(runSetID)
	if err != nil {
		return nil, nil, nil, runSetMutationRequest{}, err
	}
	if record == nil {
		return nil, nil, nil, runSetMutationRequest{}, fmt.Errorf("run-set report %q not found", runSetID)
	}
	var report acute.RunSetReport
	if err := json.Unmarshal(record.Report, &report); err != nil {
		return nil, nil, nil, runSetMutationRequest{}, fmt.Errorf("decode run-set report %q: %w", runSetID, err)
	}
	var plan acute.RunSetMutationPlan
	if err := json.Unmarshal(approval.Plan, &plan); err != nil {
		return nil, nil, nil, runSetMutationRequest{}, fmt.Errorf("decode exact plan %q: %w", runSetID, err)
	}
	var request runSetMutationRequest
	if err := json.Unmarshal(approval.Request, &request); err != nil {
		return nil, nil, nil, runSetMutationRequest{}, fmt.Errorf("decode run-set request %q: %w", runSetID, err)
	}
	return approval, &report, &plan, request, nil
}

func markRunSetApprovalStale(db *database.CatalogDB, approval *database.RunSetApprovalRecord, report *acute.RunSetReport, cause error) error {
	if err := db.ResolveRunSetApproval(approval.RunSetID, approval.PlanDigest, "stale"); err != nil {
		return fmt.Errorf("approval revalidation failed (%v), and stale transition failed: %w", cause, err)
	}
	note := "exact-plan approval became stale: " + cause.Error()
	report.ApprovalStatus = "stale"
	if err := cancelPendingRunSetMembers(db, report, note); err != nil {
		return err
	}
	if err := retainReportSnapshots(db, report, false); err != nil {
		return err
	}
	report.Status = database.RunStatusFailed
	if err := saveRunSetReport(db, report); err != nil {
		return err
	}
	return fmt.Errorf("run set %s approval is stale: %w", approval.RunSetID, cause)
}

func cancelPendingRunSetMembers(db *database.CatalogDB, report *acute.RunSetReport, note string) error {
	for index := range report.Members {
		member := &report.Members[index]
		if member.Result != database.RunStatusRunning {
			continue
		}
		if err := db.FinishRun(member.RunID, database.RunConclusion{
			CompletionStatus: database.RunStatusCancelled, Note: note,
		}); err != nil {
			return err
		}
		record, err := db.GetRunReport(member.RunID)
		if err != nil {
			return err
		}
		if record != nil {
			var memberReport RunReport
			if err := json.Unmarshal(record.Report, &memberReport); err != nil {
				return err
			}
			memberReport.PendingApproval = false
			memberReport.ApprovalStatus = report.ApprovalStatus
			memberReport.CompletionStatus = database.RunStatusCancelled
			memberReport.OK = false
			memberReport.Error = note
			if err := saveMemberRunReport(db, &memberReport); err != nil {
				return err
			}
		}
		member.Result = database.RunStatusCancelled
		member.Error = note
	}
	return nil
}

func retainReportSnapshots(db *database.CatalogDB, report *acute.RunSetReport, retain bool) error {
	seen := map[string]struct{}{}
	for _, member := range report.Members {
		record, err := db.GetRunReport(member.RunID)
		if err != nil {
			return err
		}
		if record == nil {
			continue
		}
		var memberReport RunReport
		if err := json.Unmarshal(record.Report, &memberReport); err != nil {
			return err
		}
		if memberReport.Populate != nil && memberReport.Populate.SnapshotID != "" {
			seen[memberReport.Populate.SnapshotID] = struct{}{}
		}
		for _, binding := range memberReport.Bindings {
			seen[binding.SnapshotID] = struct{}{}
		}
	}
	for snapshotID := range seen {
		if snapshotID == "" {
			continue
		}
		if err := db.RetainObserveSnapshot(snapshotID, retain); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	runSetCmd.AddCommand(runSetResumeCmd, runSetRejectCmd)
}
