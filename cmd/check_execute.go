package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
)

// checkReportVersion is the version of the one report a check records. Version
// 3 embeds each member's full report; versions 1 and 2 were `pudl run set`
// reports, whose members carried only a summary.
const checkReportVersion = 3

// checkOperation is the state a check threads through its member runs.
type checkOperation struct {
	// ctx is the invocation's lifetime. Once it is cancelled no further member
	// starts: the rest of the check is recorded cancelled.
	ctx       context.Context
	id        string
	nextRunID string
	lastRunID string
	// failures are the errors of members that did not succeed, so the check's
	// error still matches them with errors.Is.
	failures []error
}

// interrupted reports the cancellation that stops the check, or nil.
func (c *checkOperation) interrupted() error {
	if c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}

// checkCancelledNote explains a member that never started because the check
// was interrupted.
const checkCancelledNote = "cancelled: the check was interrupted before this model started"

// checkResult is what a check concluded.
type checkResult struct {
	report *acute.RunSetReport
	// findings records that a member found drift or a failing fail-severity
	// check.
	findings bool
}

// executeCheck observes and evaluates exactly the named models, producers
// first. Each member is an observe-only executeRun. The result is nil when the
// check failed before it had a report.
func executeCheck(ctx context.Context, args []string, opts checkOptions) (*checkResult, error) {
	subject := strings.Join(args, " ")
	emitRunProgress("check", subject, "started")
	result := &checkResult{}
	err := checkMembers(ctx, args, opts, result)
	state := "completed"
	if err != nil {
		state = "failed"
	}
	emitRunProgress("check", subject, state)
	if result.report == nil {
		return nil, err
	}
	return result, err
}

// checkPlan resolves the named models into a producer-first plan. An ad-hoc
// --populate check has one member named after its command.
func checkPlan(args []string, opts checkOptions) (*acute.RunSetPlan, error) {
	if opts.populateSpec != "" {
		model, _, err := adHocModel(opts.populateSpec)
		if err != nil {
			return nil, err
		}
		return acute.NewRunSetPlan([]acute.RunSetModel{{Name: model.Name}})
	}
	selected := make([]acute.RunSetModel, 0, len(args))
	for _, requested := range args {
		template, err := resolveModelTemplate(requested)
		if err != nil {
			return nil, err
		}
		selected = append(selected, acute.RunSetModel{
			Name: template.Name, DependsOn: template.DependsOn,
			Aliases: []string{requested, systemmodel.ShortDefName(template.Origin.SchemaName)},
		})
	}
	return acute.NewRunSetPlan(selected)
}

func checkMembers(ctx context.Context, args []string, opts checkOptions, result *checkResult) error {
	if err := validateRunFlags(opts.runFlags); err != nil {
		return err
	}
	plan, err := checkPlan(args, opts)
	if err != nil {
		return err
	}
	mode := "observe-only"
	digest, err := plan.Digest(mode)
	if err != nil {
		return err
	}

	db, err := database.NewCatalogDB(effectivePudlDir())
	if err != nil {
		return fmt.Errorf("open catalog: %w", err)
	}
	defer db.Close()
	report := &acute.RunSetReport{
		RunSetID: acute.NewCheckID(), Mode: mode,
		Status: "running", PlanDigest: digest, Edges: plan.Edges, Ordered: plan.Ordered,
	}
	if err := saveCheckReport(db, report); err != nil {
		return err
	}
	result.report = report

	op := &checkOperation{ctx: ctx, id: report.RunSetID}
	results := map[string]string{}
	for _, model := range plan.Ordered {
		member, findings, err := checkMember(db, op, opts, model, plan.Edges, results)
		if err != nil {
			return err
		}
		result.findings = result.findings || findings
		results[model] = member.Result
		report.Members = append(report.Members, member)
		if err := saveCheckReport(db, report); err != nil {
			return err
		}
	}

	report.Status = checkStatus(ctx, report)
	if err := saveCheckReport(db, report); err != nil {
		return err
	}
	if err := printCheckReport(report, false); err != nil {
		return err
	}
	switch report.Status {
	case database.RunStatusSucceeded:
		return nil
	case database.RunStatusCancelled:
		return fmt.Errorf("check %s %s: %w", report.RunSetID, database.RunStatusCancelled, ctx.Err())
	default:
		return fmt.Errorf("check %s failed: %w", report.RunSetID, errors.Join(op.failures...))
	}
}

// checkMember runs one member — or records why it did not run — and returns
// its entry in the check report. err aborts the whole check.
func checkMember(db *database.CatalogDB, op *checkOperation, opts checkOptions, model string, edges []acute.RunSetEdge, results map[string]string) (acute.RunSetMemberReport, bool, error) {
	synthetic := func(status, note string) (acute.RunSetMemberReport, bool, error) {
		runID := acute.NewMemberRunID()
		op.failures = append(op.failures, fmt.Errorf("%s %s: %s", model, status, note))
		err := recordSyntheticMember(db, op.id, runID, model, status, note)
		return acute.RunSetMemberReport{Model: model, RunID: runID, Result: status, Error: note}, false, err
	}
	if op.interrupted() != nil {
		return synthetic(database.RunStatusCancelled, checkCancelledNote)
	}
	if blocker := firstNonSuccessfulPrerequisite(model, edges, results); blocker != "" {
		return synthetic(database.RunStatusBlocked, fmt.Sprintf("blocked by unsuccessful prerequisite %q", blocker))
	}

	op.nextRunID = acute.NewMemberRunID()
	op.lastRunID = ""
	memberReport, runErr := executeRun(op.ctx, opts.memberRunOptions(model), op)
	findings := reportHasFindings(memberReport)
	if op.lastRunID == "" {
		if runErr == nil {
			runErr = fmt.Errorf("model failed before it started")
		}
		runID := acute.NewMemberRunID()
		op.failures = append(op.failures, fmt.Errorf("%s %s: %w", model, database.RunStatusFailed, runErr))
		err := recordSyntheticMember(db, op.id, runID, model, database.RunStatusFailed, runErr.Error())
		return acute.RunSetMemberReport{Model: model, RunID: runID, Result: database.RunStatusFailed, Error: runErr.Error()}, findings, err
	}
	member := acute.RunSetMemberReport{Model: model, RunID: op.lastRunID, Result: database.RunStatusSucceeded}
	if runErr != nil {
		member.Result = failureStatus(runErr)
		member.Error = runErr.Error()
		op.failures = append(op.failures, fmt.Errorf("%s %s: %w", model, member.Result, runErr))
	}
	return member, findings, nil
}

func firstNonSuccessfulPrerequisite(model string, edges []acute.RunSetEdge, results map[string]string) string {
	for _, edge := range edges {
		if edge.From == model && results[edge.To] != database.RunStatusSucceeded {
			return edge.To
		}
	}
	return ""
}

// recordSyntheticMember records the run row and report of a member that never
// ran its phases: cancelled, blocked or failed before it started.
func recordSyntheticMember(db *database.CatalogDB, checkID, runID, model, status, note string) error {
	if err := db.StartRun(runID, model, "observe-only"); err != nil {
		return err
	}
	if err := db.FinishRun(runID, database.RunConclusion{CompletionStatus: status, Note: note}); err != nil {
		return err
	}
	report := &checkreport.ModelReport{
		ReportVersion: 2, RunSetID: checkID, RunID: runID, Model: model,
		Mode: "observe-only", CompletionStatus: status, OK: false, Error: note,
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return db.SaveRunReport(runID, model, payload)
}

// saveCheckReport summarizes the members recorded so far, embeds their full
// reports and persists the check report.
func saveCheckReport(db *database.CatalogDB, report *acute.RunSetReport) error {
	report.ReportVersion = checkReportVersion
	if err := checkreport.SummarizeCheck(db, report); err != nil {
		return err
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return db.SaveRunSetReport(report.RunSetID, payload)
}

// checkStatus is a finished check's status: succeeded only when every member
// succeeded, cancelled when an interrupt is why it did not, failed otherwise.
func checkStatus(ctx context.Context, report *acute.RunSetReport) string {
	for _, member := range report.Members {
		if member.Result != database.RunStatusSucceeded {
			if ctx != nil && ctx.Err() != nil {
				return database.RunStatusCancelled
			}
			return database.RunStatusFailed
		}
	}
	return database.RunStatusSucceeded
}
