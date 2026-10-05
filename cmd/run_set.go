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

// runSetExecutionContext is the coordinator state a run set threads through its
// member runs. A standalone run carries a nil context; every method is nil-safe
// so the run path reads it without branching on whether it is a set member.
type runSetExecutionContext struct {
	runSetID         string
	mu               muRunner
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

// id is the run set's ID, or "" for a standalone run.
func (c *runSetExecutionContext) id() string {
	if c == nil {
		return ""
	}
	return c.runSetID
}

// producerRuns are the members that already succeeded in this set.
func (c *runSetExecutionContext) producerRuns() map[string]wiring.ProducerRun {
	if c == nil {
		return nil
	}
	return c.successfulRuns
}

// memberRunID is the run ID reserved for the member about to run.
func (c *runSetExecutionContext) memberRunID() string {
	if c == nil {
		return ""
	}
	return c.nextRunID
}

// resolveSealedModel resolves sealed sources for the current member against
// the members that already succeeded; a standalone run has none to resolve.
func (c *runSetExecutionContext) resolveSealedModel(model *systemmodel.SystemModel) (*systemmodel.SystemModel, []wiring.SealedBindingEvidence, error) {
	if c == nil {
		return model, nil, nil
	}
	members := make([]wiring.SealedMember, 0, len(c.successfulModels)+1)
	for name, successful := range c.successfulModels {
		producer := c.successfulRuns[name]
		members = append(members, wiring.SealedMember{
			Model: successful, Aliases: c.aliases[name], RunID: producer.RunID,
		})
	}
	members = append(members, wiring.SealedMember{
		Model: model, Aliases: c.aliases[model.Name], RunID: c.nextRunID,
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

// registerMemberRunID records the run ID the member actually used.
func (c *runSetExecutionContext) registerMemberRunID(runID string) {
	if c != nil {
		c.lastRunID = runID
	}
}

// retainMember keeps the elaborated member so downstream members and mutation
// planning can use it once the member succeeds.
func (c *runSetExecutionContext) retainMember(model *systemmodel.SystemModel, sealed []wiring.SealedBindingEvidence, snapshotID, modelDir string) {
	if c == nil {
		return
	}
	c.lastModel = model
	c.lastSealed = sealed
	c.lastSnapshotID = snapshotID
	c.lastModelDir = modelDir
}

// emitOutput reports whether a member run prints its own report.
func (c *runSetExecutionContext) emitOutput() bool {
	return c == nil || !c.suppressOutput
}

var (
	runSetMaxObservationAge time.Duration
	runSetMuRoot            string
	runSetConverge          bool
	runSetRequireApproval   bool
	runSetMaxIters          int
	runSetMaxApplies        int
)

// runSetOptions is the complete input of one `pudl run set`, read once from
// its flags.
type runSetOptions struct {
	muRoot               string
	converge             bool
	requireApproval      bool
	maxIters             int
	maxApplies           int
	maxObservationAge    time.Duration
	maxObservationAgeSet bool
}

func runSetOptionsFromFlags(cmd *cobra.Command) runSetOptions {
	return runSetOptions{
		muRoot: runSetMuRoot, converge: runSetConverge, requireApproval: runSetRequireApproval,
		maxIters: runSetMaxIters, maxApplies: runSetMaxApplies,
		maxObservationAge:    runSetMaxObservationAge,
		maxObservationAgeSet: cmd.Flags().Changed("max-observation-age"),
	}
}

var runSetCmd = &cobra.Command{
	Use:   "set <model> [<model>...]",
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
  pudl run set network app
  pudl run set network app --max-observation-age 15m
  pudl run set network app --converge
  pudl run set network app --converge --require-approval
  pudl run report
  pudl run resume <run-set-id>
  pudl run reject <run-set-id>`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeRunSet(args, runSetOptionsFromFlags(cmd), defaultRunDeps())
	},
}

// executeRunSet runs exactly the named models in dependency order. Each member
// is an observe-only executeRun with its own options; a converging set then
// plans and executes mutations across the whole set.
func executeRunSet(args []string, opts runSetOptions, deps runDeps) error {
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
	if opts.requireApproval && !opts.converge {
		return fmt.Errorf("--require-approval requires --converge")
	}
	if opts.converge && opts.maxIters < 1 {
		return fmt.Errorf("--max-iters must be >= 1")
	}
	if opts.maxApplies < 0 {
		return fmt.Errorf("--max-applies must be >= 0")
	}
	plan, err := acute.NewRunSetPlan(selected)
	if err != nil {
		return err
	}
	if opts.maxObservationAgeSet && opts.maxObservationAge <= 0 {
		return fmt.Errorf("--max-observation-age must be greater than zero")
	}
	agePolicy := ""
	if opts.maxObservationAgeSet {
		agePolicy = opts.maxObservationAge.String()
	}
	mode := "observe-only"
	if opts.converge {
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
		runSetID: report.RunSetID, mu: deps.mu, successfulRuns: map[string]wiring.ProducerRun{},
		successfulModels: map[string]*systemmodel.SystemModel{},
		snapshotIDs:      map[string]string{}, modelDirs: map[string]string{},
		bindingEvidence: map[string][]wiring.BindingEvidence{},
		sealedEvidence:  map[string][]wiring.SealedBindingEvidence{}, aliases: aliases,
		suppressOutput: jsonOutput,
	}
	memberDeps := runDeps{mu: deps.mu, set: context}

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
		_, runErr := executeRun(memberRunOptions(model, opts), memberDeps)
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
	if report.Status == database.RunStatusSucceeded && opts.converge {
		return continueMutatingRunSet(db, plan, report, context, runSetMutationRequest{
			Models: append([]string(nil), args...), MaxObservationAge: agePolicy,
			MaxIterations: opts.maxIters, MaxApplies: opts.maxApplies,
			MuRoot: opts.muRoot, RequireApproval: opts.requireApproval || hasSealedOutputs,
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
	runCmd.AddCommand(runSetCmd)
	runSetCmd.Flags().DurationVar(&runSetMaxObservationAge, "max-observation-age", 0, "reject a bound producer snapshot older than this duration")
	runSetCmd.Flags().StringVar(&runSetMuRoot, "mu-root", "", "mu project root for member runs (default: discover per model)")
	runSetCmd.Flags().BoolVar(&runSetConverge, "converge", false, "plan and execute mutations only after every member completes read-only preflight")
	runSetCmd.Flags().BoolVar(&runSetRequireApproval, "require-approval", false, "persist the exact run-set plan and wait for approval before mutation")
	runSetCmd.Flags().IntVar(&runSetMaxIters, "max-iters", defaultRunMaxIters, "maximum apply iterations per mutating member")
	runSetCmd.Flags().IntVar(&runSetMaxApplies, "max-applies", defaultRunMaxApplies, "durable apply budget per mutating member (0 disables)")
}
