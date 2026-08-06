package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

type runSetExecutionContext struct {
	runSetID         string
	successfulRuns   map[string]wiring.ProducerRun
	successfulModels map[string]*systemmodel.SystemModel
	snapshotIDs      map[string]string
	modelDirs        map[string]string
	bindingEvidence  map[string][]wiring.BindingEvidence
	sealedEvidence   map[string][]wiring.SealedBindingEvidence
	aliases          map[string][]string
	nextRunID        string
	lastModel        *systemmodel.SystemModel
	lastSealed       []wiring.SealedBindingEvidence
	lastSnapshotID   string
	lastModelDir     string
	lastRunID        string
	suppressOutput   bool
}

var activeRunSet *runSetExecutionContext

func currentRunSetID() string {
	if activeRunSet == nil {
		return ""
	}
	return activeRunSet.runSetID
}

func currentRunSetProducerRuns() map[string]wiring.ProducerRun {
	if activeRunSet == nil {
		return nil
	}
	return activeRunSet.successfulRuns
}

func currentRunSetMemberRunID() string {
	if activeRunSet == nil {
		return ""
	}
	return activeRunSet.nextRunID
}

func resolveCurrentRunSetSealedModel(model *systemmodel.SystemModel) (*systemmodel.SystemModel, []wiring.SealedBindingEvidence, error) {
	if activeRunSet == nil {
		return model, nil, nil
	}
	members := make([]wiring.SealedMember, 0, len(activeRunSet.successfulModels)+1)
	for name, successful := range activeRunSet.successfulModels {
		producer := activeRunSet.successfulRuns[name]
		members = append(members, wiring.SealedMember{
			Model: successful, Aliases: activeRunSet.aliases[name], RunID: producer.RunID,
		})
	}
	members = append(members, wiring.SealedMember{
		Model: model, Aliases: activeRunSet.aliases[model.Name], RunID: activeRunSet.nextRunID,
	})
	refs, configured := wsPolicy.SecretsWritablePolicy()
	resolved, err := wiring.ResolveSealedSources(members, wiring.SealedPolicy{
		WritableRefs: refs, WritableConfigured: configured,
	})
	if err != nil {
		return nil, nil, err
	}
	for _, member := range resolved {
		if member.Model.Name == model.Name {
			return member.Model, member.Evidence, nil
		}
	}
	return nil, nil, fmt.Errorf("sealed resolution omitted current model %q", model.Name)
}

func registerRunSetMemberRunID(runID string) {
	if activeRunSet != nil {
		activeRunSet.lastRunID = runID
	}
}

func emitRunSetMemberOutput() bool {
	return activeRunSet == nil || !activeRunSet.suppressOutput
}

var (
	runSetMaxObservationAge time.Duration
	runSetMuRoot            string
	runSetConverge          bool
	runSetRequireApproval   bool
	runSetMaxIters          int
	runSetMaxApplies        int
)

var runSetCmd = &cobra.Command{
	Use:   "run-set <model> [<model>...]",
	Short: "Run an explicit producer/consumer model set in dependency order",
	Long: `Run exactly the named models in dependency order.

The set is closed and explicit: pudl does not add missing producers. A binding
whose producer is not named fails preflight before any member runs. Without
--converge, every member is observe-only and successful producer snapshots are
pinned for downstream plain bindings.

With --converge, pudl completes read-only preflight and exact mu planning for
the whole set before the first mutation. --require-approval persists and pauses
any exact plan. A set that can write a sealed output is always approval-gated,
even without the flag. Generated targets use strict sealed routing, so unused
declarations, undeclared action claims, and ambiguous output writers fail during
planning before mutation or provider traffic. Resume rebuilds and revalidates
the exact plan before producer-first execution.

Examples:
  pudl run-set network app
  pudl run-set network app --max-observation-age 15m
  pudl run-set network app --converge
  pudl run-set network app --converge --require-approval
  pudl run-set report
  pudl run-set resume <run-set-id>
  pudl run-set reject <run-set-id>`,
	Args: cobra.MinimumNArgs(1),
	RunE: runObserveSet,
}

func runObserveSet(cmd *cobra.Command, args []string) error {
	selected := make([]acute.RunSetModel, 0, len(args))
	hasSealedOutputs := false
	for _, requested := range args {
		template, _, _, err := resolveModelTemplate(requested)
		if err != nil {
			return err
		}
		selected = append(selected, acute.RunSetModel{
			Template: template,
			Aliases:  []string{requested, shortDefName(template.Origin.SchemaName)},
		})
		hasSealedOutputs = hasSealedOutputs || template.HasSealedOutputs()
	}
	if runSetRequireApproval && !runSetConverge {
		return fmt.Errorf("--require-approval requires --converge")
	}
	if runSetConverge && runSetMaxIters < 1 {
		return fmt.Errorf("--max-iters must be >= 1")
	}
	if runSetMaxApplies < 0 {
		return fmt.Errorf("--max-applies must be >= 0")
	}
	plan, err := acute.NewRunSetPlan(selected)
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("max-observation-age") && runSetMaxObservationAge <= 0 {
		return fmt.Errorf("--max-observation-age must be greater than zero")
	}
	agePolicy := ""
	if cmd.Flags().Changed("max-observation-age") {
		agePolicy = runSetMaxObservationAge.String()
	}
	mode := "observe-only"
	if runSetConverge {
		mode = "converge"
	}
	digest, err := plan.Digest(mode, agePolicy)
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

	aliases := make(map[string][]string, len(plan.Models))
	for name, member := range plan.Models {
		aliases[name] = append([]string(nil), member.Aliases...)
	}
	context := &runSetExecutionContext{
		runSetID: report.RunSetID, successfulRuns: map[string]wiring.ProducerRun{},
		successfulModels: map[string]*systemmodel.SystemModel{},
		snapshotIDs:      map[string]string{}, modelDirs: map[string]string{},
		bindingEvidence: map[string][]wiring.BindingEvidence{},
		sealedEvidence:  map[string][]wiring.SealedBindingEvidence{}, aliases: aliases,
		suppressOutput: jsonOutput,
	}
	activeRunSet = context
	defer func() { activeRunSet = nil }()
	restore := configureMemberRunGlobals(cmd)
	defer restore()

	results := map[string]string{}
	for _, model := range plan.Ordered {
		if blocker := firstNonSuccessfulPrerequisite(model, plan.Edges, results); blocker != "" {
			runID := acute.NewMemberRunID()
			note := fmt.Sprintf("blocked by unsuccessful prerequisite %q", blocker)
			issues := blockedBindingIssues(plan.Models[model].Template, blocker, plan.Models[blocker].Aliases)
			if err := recordSyntheticRunSetMember(db, report.RunSetID, runID, model, database.RunStatusBlocked, note, issues); err != nil {
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
		context.lastModel = nil
		context.lastSealed = nil
		context.lastSnapshotID = ""
		context.lastModelDir = ""
		runErr := runCmd.RunE(cmd, []string{model})
		runID := context.lastRunID
		if runID == "" {
			runID = acute.NewMemberRunID()
			note := "member failed during preflight"
			if runErr != nil {
				note = runErr.Error()
			}
			issues := resolutionBindingIssues(plan.Models[model].Template, runErr)
			if err := recordSyntheticRunSetMember(db, report.RunSetID, runID, model, database.RunStatusFailed, note, issues); err != nil {
				return err
			}
		}
		member := acute.RunSetMemberReport{Model: model, RunID: runID}
		if runErr != nil {
			member.Result = database.RunStatusFailed
			member.Error = runErr.Error()
			results[model] = database.RunStatusFailed
		} else {
			member.Result = database.RunStatusSucceeded
			results[model] = database.RunStatusSucceeded
			producerRun := wiring.ProducerRun{Model: model, RunID: runID}
			context.successfulRuns[model] = producerRun
			if context.lastModel == nil {
				return fmt.Errorf("run-set member %q succeeded without retaining its elaborated model", model)
			}
			context.successfulModels[model] = context.lastModel
			context.snapshotIDs[model] = context.lastSnapshotID
			context.modelDirs[model] = context.lastModelDir
			context.sealedEvidence[model] = append([]wiring.SealedBindingEvidence(nil), context.lastSealed...)
			for _, alias := range plan.Models[model].Aliases {
				context.successfulRuns[alias] = producerRun
			}
		}
		report.Members = append(report.Members, member)
		if err := saveRunSetReport(db, report); err != nil {
			return err
		}
	}

	report.Status = database.RunStatusSucceeded
	for _, member := range report.Members {
		if member.Result != database.RunStatusSucceeded {
			report.Status = database.RunStatusFailed
			break
		}
	}
	if report.Status == database.RunStatusSucceeded && runSetConverge {
		return continueMutatingRunSet(db, plan, report, context, runSetMutationRequest{
			Models: append([]string(nil), args...), MaxObservationAge: agePolicy,
			MaxIterations: runSetMaxIters, MaxApplies: runSetMaxApplies,
			MuRoot: runSetMuRoot, RequireApproval: runSetRequireApproval || hasSealedOutputs,
		})
	}
	if err := saveRunSetReport(db, report); err != nil {
		return err
	}
	if err := printRunSetReport(report); err != nil {
		return err
	}
	if report.Status != database.RunStatusSucceeded {
		return fmt.Errorf("run set %s failed", report.RunSetID)
	}
	return nil
}

func configureMemberRunGlobals(cmd *cobra.Command) func() {
	type state struct {
		muRoot, populateSpec, resumeID                                string
		converge, dryRun, fromCatalog, checkUpstream, requireApproval bool
		only, populateInput                                           []string
		maxIters, maxApplies                                          int
		catalogScope                                                  string
		maxAge                                                        time.Duration
	}
	before := state{
		muRoot: runMuRoot, populateSpec: runPopulateSpec, resumeID: runResumeID,
		converge: runConverge, dryRun: runDryRun, fromCatalog: runFromCatalog,
		checkUpstream: runCheckUpstream, requireApproval: runRequireApproval,
		only: runOnly, populateInput: runPopulateInput, maxIters: runMaxIters,
		maxApplies: runMaxApplies, catalogScope: runCatalogScope, maxAge: runMaxObservationAge,
	}
	runMuRoot, runPopulateSpec, runResumeID = runSetMuRoot, "", ""
	runConverge, runDryRun, runFromCatalog = false, false, false
	runCheckUpstream, runRequireApproval = false, false
	runOnly, runPopulateInput = nil, nil
	runMaxIters, runMaxApplies, runCatalogScope = 5, 20, ""
	runMaxObservationAge = runSetMaxObservationAge
	return func() {
		runMuRoot, runPopulateSpec, runResumeID = before.muRoot, before.populateSpec, before.resumeID
		runConverge, runDryRun, runFromCatalog = before.converge, before.dryRun, before.fromCatalog
		runCheckUpstream, runRequireApproval = before.checkUpstream, before.requireApproval
		runOnly, runPopulateInput = before.only, before.populateInput
		runMaxIters, runMaxApplies, runCatalogScope = before.maxIters, before.maxApplies, before.catalogScope
		runMaxObservationAge = before.maxAge
	}
}

func firstNonSuccessfulPrerequisite(model string, edges []acute.RunSetEdge, results map[string]string) string {
	for _, edge := range edges {
		if edge.From == model && results[edge.To] != database.RunStatusSucceeded {
			return edge.To
		}
	}
	return ""
}

func recordSyntheticRunSetMember(db *database.CatalogDB, runSetID, runID, model, status, note string, issues []wiring.BindingIssue) error {
	if err := db.StartRun(runID, model, "run-set observe-only"); err != nil {
		return err
	}
	if err := db.FinishRun(runID, database.RunConclusion{CompletionStatus: status, Note: note}); err != nil {
		return err
	}
	report := &RunReport{
		ReportVersion: 1, RunSetID: runSetID, RunID: runID, Model: model,
		Mode: "observe-only", CompletionStatus: status, OK: false, Error: note,
		BindingIssues: issues,
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return db.SaveRunReport(runID, model, payload)
}

func blockedBindingIssues(template *systemmodel.ModelTemplate, blocker string, aliases []string) []wiring.BindingIssue {
	if template == nil {
		return nil
	}
	producerNames := map[string]struct{}{blocker: {}}
	for _, alias := range aliases {
		producerNames[alias] = struct{}{}
	}
	issues := make([]wiring.BindingIssue, 0)
	for input, binding := range template.Bindings {
		if _, matches := producerNames[binding.Source.Model]; !matches {
			continue
		}
		issues = append(issues, wiring.BindingIssue{
			Input: input, ProducerModel: blocker, Schema: binding.Source.Schema,
			Identity: binding.Source.Identity, Path: binding.Path,
			Code: "producer-unsuccessful", Message: fmt.Sprintf("producer %q did not complete successfully in this run set; historical fallback is forbidden", blocker),
		})
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Input < issues[j].Input })
	return issues
}

func resolutionBindingIssues(template *systemmodel.ModelTemplate, err error) []wiring.BindingIssue {
	if template == nil || err == nil {
		return nil
	}
	var resolution *wiring.ResolutionError
	if !errors.As(err, &resolution) {
		return nil
	}
	issue := wiring.BindingIssue{
		Input: resolution.Input, Code: resolution.Code, Message: resolution.Error(),
	}
	if binding, exists := template.Bindings[resolution.Input]; exists {
		issue.ProducerModel = binding.Source.Model
		issue.Schema = binding.Source.Schema
		issue.Identity = binding.Source.Identity
		issue.Path = binding.Path
	}
	return []wiring.BindingIssue{issue}
}

func saveRunSetReport(db *database.CatalogDB, report *acute.RunSetReport) error {
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
		fmt.Println(string(payload))
		return nil
	}
	fmt.Printf("\nrun-set %s: %s\n", report.RunSetID, report.Status)
	fmt.Printf("plan: %s\n", report.PlanDigest)
	for _, member := range report.Members {
		line := fmt.Sprintf("  %s: %s (%s)", member.Model, member.Result, member.RunID)
		if member.Error != "" {
			line += ": " + member.Error
		}
		fmt.Println(strings.TrimSpace(line))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(runSetCmd)
	runSetCmd.Flags().DurationVar(&runSetMaxObservationAge, "max-observation-age", 0, "reject a bound producer snapshot older than this duration")
	runSetCmd.Flags().StringVar(&runSetMuRoot, "mu-root", "", "mu project root for member runs (default: discover per model)")
	runSetCmd.Flags().BoolVar(&runSetConverge, "converge", false, "plan and execute mutations only after every member completes read-only preflight")
	runSetCmd.Flags().BoolVar(&runSetRequireApproval, "require-approval", false, "persist the exact run-set plan and wait for approval before mutation")
	runSetCmd.Flags().IntVar(&runSetMaxIters, "max-iters", 5, "maximum apply iterations per mutating member")
	runSetCmd.Flags().IntVar(&runSetMaxApplies, "max-applies", 20, "durable apply budget per mutating member (0 disables)")
}
