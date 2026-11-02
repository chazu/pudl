package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
)

// runSetExecutionContext is the coordinator state a run set threads through its
// member runs. A standalone run carries a nil context; every method is nil-safe
// so the run path reads it without branching on whether it is a set member.
type runSetExecutionContext struct {
	// ctx is the invocation's lifetime. Once it is cancelled no further member
	// starts: the rest of the set is recorded cancelled.
	ctx            context.Context
	runSetID       string
	nextRunID      string
	lastRunID      string
	suppressOutput bool
}

// interrupted reports the cancellation that stops the set, or nil.
func (c *runSetExecutionContext) interrupted() error {
	if c == nil || c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}

// runSetCancelledNote explains a member that never started because the set was
// interrupted.
const runSetCancelledNote = "cancelled: the run set was interrupted before this member started"

// id is the run set's ID, or "" for a standalone run.
func (c *runSetExecutionContext) id() string {
	if c == nil {
		return ""
	}
	return c.runSetID
}

// memberRunID is the run ID reserved for the member about to run.
func (c *runSetExecutionContext) memberRunID() string {
	if c == nil {
		return ""
	}
	return c.nextRunID
}

// registerMemberRunID records the run ID the member actually used.
func (c *runSetExecutionContext) registerMemberRunID(runID string) {
	if c != nil {
		c.lastRunID = runID
	}
}

// emitOutput reports whether a member run prints its own report.
func (c *runSetExecutionContext) emitOutput() bool {
	return c == nil || !c.suppressOutput
}

var runSetDetailedExitCode bool

var runSetCmd = &cobra.Command{
	Use:   "set <model> [<model>...]",
	Short: "Run an explicit producer/consumer model set in dependency order",
	Long: `Run exactly the named models in dependency order.

The set is closed and explicit: pudl does not add missing producers. Members
run producers first, following depends_on within the named set. Every member
is observe-only.

Examples:
  pudl run set network app
  pudl run set network app --detailed-exitcode
  pudl run report`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := executeRunSet(cmd.Context(), args)
		if !runSetDetailedExitCode {
			return err
		}
		return silenceBareExit(cmd, detailedRunSetExit(result, err))
	},
}

// runSetResult is what a run set concluded.
type runSetResult struct {
	report *acute.RunSetReport
	// findings records that an observe-only member found drift, pending
	// changes or a failing fail-severity check.
	findings bool
}

// executeRunSet runs exactly the named models in dependency order. Each member
// is an observe-only executeRun with its own options. The result is nil when
// the set failed before it had a report.
func executeRunSet(ctx context.Context, args []string) (*runSetResult, error) {
	result := &runSetResult{}
	err := runSetMembers(ctx, args, result)
	if result.report == nil {
		return nil, err
	}
	return result, err
}

func runSetMembers(ctx context.Context, args []string, result *runSetResult) error {
	selected := make([]acute.RunSetModel, 0, len(args))
	for _, requested := range args {
		template, _, _, err := resolveModelTemplate(requested)
		if err != nil {
			return err
		}
		selected = append(selected, acute.RunSetModel{
			Name: template.Name, DependsOn: template.DependsOn,
			Aliases: []string{requested, shortDefName(template.Origin.SchemaName)},
		})
	}
	plan, err := acute.NewRunSetPlan(selected)
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
		ReportVersion: 1, RunSetID: acute.NewRunSetID(), Mode: mode,
		Status: "running", PlanDigest: digest, Edges: plan.Edges, Ordered: plan.Ordered,
	}
	if err := saveRunSetReport(db, report); err != nil {
		return err
	}
	result.report = report

	context := &runSetExecutionContext{
		ctx:      ctx,
		runSetID: report.RunSetID, suppressOutput: jsonOutput,
	}
	memberDeps := runDeps{set: context}

	results := map[string]string{}
	for _, model := range plan.Ordered {
		if context.interrupted() != nil {
			runID := acute.NewMemberRunID()
			if err := recordSyntheticRunSetMember(db, report.RunSetID, runID, model, database.RunStatusCancelled, runSetCancelledNote); err != nil {
				return err
			}
			results[model] = database.RunStatusCancelled
			report.Members = append(report.Members, acute.RunSetMemberReport{Model: model, RunID: runID, Result: database.RunStatusCancelled, Error: runSetCancelledNote})
			if err := saveRunSetReport(db, report); err != nil {
				return err
			}
			continue
		}
		if blocker := firstNonSuccessfulPrerequisite(model, plan.Edges, results); blocker != "" {
			runID := acute.NewMemberRunID()
			note := fmt.Sprintf("blocked by unsuccessful prerequisite %q", blocker)
			if err := recordSyntheticRunSetMember(db, report.RunSetID, runID, model, database.RunStatusBlocked, note); err != nil {
				return err
			}
			results[model] = database.RunStatusBlocked
			report.Members = append(report.Members, acute.RunSetMemberReport{Model: model, RunID: runID, Result: database.RunStatusBlocked, Error: note})
			if err := saveRunSetReport(db, report); err != nil {
				return err
			}
			continue
		}

		context.nextRunID = acute.NewMemberRunID()
		context.lastRunID = ""
		memberReport, runErr := executeRun(ctx, runOptions{model: model}, memberDeps)
		result.findings = result.findings || reportHasFindings(memberReport)
		runID := context.lastRunID
		if runID == "" {
			runID = acute.NewMemberRunID()
			note := "member failed during preflight"
			if runErr != nil {
				note = runErr.Error()
			}
			if err := recordSyntheticRunSetMember(db, report.RunSetID, runID, model, database.RunStatusFailed, note); err != nil {
				return err
			}
		}
		member := acute.RunSetMemberReport{Model: model, RunID: runID}
		if runErr != nil {
			member.Result = failureStatus(runErr)
			member.Error = runErr.Error()
			results[model] = member.Result
		} else {
			member.Result = database.RunStatusSucceeded
			results[model] = database.RunStatusSucceeded
		}
		report.Members = append(report.Members, member)
		if err := saveRunSetReport(db, report); err != nil {
			return err
		}
	}

	report.Status = runSetStatus(ctx, report)
	if err := saveRunSetReport(db, report); err != nil {
		return err
	}
	if err := printRunSetReport(report); err != nil {
		return err
	}
	if report.Status == database.RunStatusCancelled {
		return fmt.Errorf("run set %s %s: %w", report.RunSetID, database.RunStatusCancelled, ctx.Err())
	}
	if report.Status != database.RunStatusSucceeded {
		return fmt.Errorf("run set %s failed", report.RunSetID)
	}
	return nil
}

func firstNonSuccessfulPrerequisite(model string, edges []acute.RunSetEdge, results map[string]string) string {
	for _, edge := range edges {
		if edge.From == model && results[edge.To] != database.RunStatusSucceeded {
			return edge.To
		}
	}
	return ""
}

func recordSyntheticRunSetMember(db *database.CatalogDB, runSetID, runID, model, status, note string) error {
	if err := db.StartRun(runID, model, "run-set observe-only"); err != nil {
		return err
	}
	if err := db.FinishRun(runID, database.RunConclusion{CompletionStatus: status, Note: note}); err != nil {
		return err
	}
	report := &RunReport{
		ReportVersion: 1, RunSetID: runSetID, RunID: runID, Model: model,
		Mode: "observe-only", CompletionStatus: status, OK: false, Error: note,
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return db.SaveRunReport(runID, model, payload)
}

func saveRunSetReport(db *database.CatalogDB, report *acute.RunSetReport) error {
	report.ReportVersion = 2
	if err := enrichSetSummary(db, report); err != nil {
		return err
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return db.SaveRunSetReport(report.RunSetID, payload)
}

func printRunSetReport(report *acute.RunSetReport) error {
	if jsonOutput {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(outw(), string(payload))
		return err
	}
	fmt.Fprintf(outw(), "\nrun-set %s: %s\n", report.RunSetID, report.Status)
	if s := report.Summary; s != nil {
		fmt.Fprintf(outw(), "conformity: %s | checks: %s | verification: %s\n", s.Conformity, s.Checks, s.Verification)
	}
	fmt.Fprintf(outw(), "plan: %s\n", report.PlanDigest)
	for _, member := range report.Members {
		line := fmt.Sprintf("  %s: %s (%s)", member.Model, member.Result, member.RunID)
		if s := member.Summary; s != nil {
			line += fmt.Sprintf(" | conformity: %s | checks: %s | verification: %s | scope: %s", s.Conformity, s.Checks, s.Verification, s.Scope)
		}
		if member.Error != "" {
			line += ": " + member.Error
		}
		fmt.Fprintln(outw(), strings.TrimSpace(line))
		if member.Summary != nil {
			for _, e := range member.Summary.Evidence {
				fmt.Fprintf(outw(), "    evidence: %s%s | age at check: %s | complete: %t\n", e.SnapshotID, e.ObservationID, e.Age, e.Complete)
			}
		}
	}
	return nil
}

func init() {
	runCmd.AddCommand(runSetCmd)
	runSetCmd.Flags().BoolVar(&runSetDetailedExitCode, "detailed-exitcode", false, detailedExitCodeUsage)
}

// runSetStatus is a finished set's status: succeeded only when every member
// succeeded, cancelled when an interrupt is why it did not, failed otherwise.
func runSetStatus(ctx context.Context, report *acute.RunSetReport) string {
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
