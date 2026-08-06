package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

type runSetMutationRequest struct {
	Models            []string `json:"models"`
	MaxObservationAge string   `json:"max_observation_age,omitempty"`
	MaxIterations     int      `json:"max_iterations"`
	MaxApplies        int      `json:"max_applies"`
	MuRoot            string   `json:"mu_root,omitempty"`
	RequireApproval   bool     `json:"require_approval"`
}

type preparedMutationMember struct {
	model      *systemmodel.SystemModel
	report     *RunReport
	runID      string
	snapshotID string
	modelDir   string
	muRoot     string
	required   bool
	// expectedMuPlanSHA256 is the workspace-normalized mu plan identity that
	// was included in the approved run-set mutation plan.
	expectedMuPlanSHA256 string
}

func reconstructApprovedRunSet(db *database.CatalogDB, report *acute.RunSetReport, stored *acute.RunSetMutationPlan, request runSetMutationRequest) (*acute.RunSetPlan, *runSetExecutionContext, error) {
	selected := make([]acute.RunSetModel, 0, len(request.Models))
	for _, requested := range request.Models {
		template, _, _, err := resolveModelTemplate(requested)
		if err != nil {
			return nil, nil, err
		}
		selected = append(selected, acute.RunSetModel{
			Template: template, Aliases: []string{requested, shortDefName(template.Origin.SchemaName)},
		})
	}
	graph, err := acute.NewRunSetPlan(selected)
	if err != nil {
		return nil, nil, err
	}
	storedMembers := make(map[string]acute.RunSetMutationMemberPlan, len(stored.Members))
	for _, member := range stored.Members {
		storedMembers[member.Model] = member
	}
	aliases := make(map[string][]string, len(graph.Models))
	pinned := map[string]wiring.PinnedProducerSnapshot{}
	context := &runSetExecutionContext{
		runSetID: report.RunSetID, successfulRuns: map[string]wiring.ProducerRun{},
		successfulModels: map[string]*systemmodel.SystemModel{}, snapshotIDs: map[string]string{},
		modelDirs: map[string]string{}, bindingEvidence: map[string][]wiring.BindingEvidence{},
		sealedEvidence: map[string][]wiring.SealedBindingEvidence{}, aliases: aliases,
	}
	for name, graphMember := range graph.Models {
		storedMember, exists := storedMembers[name]
		if !exists {
			return nil, nil, fmt.Errorf("stored exact plan has no member %q", name)
		}
		memberAliases := append([]string(nil), graphMember.Aliases...)
		aliases[name] = memberAliases
		producerRun := wiring.ProducerRun{Model: name, RunID: storedMember.RunID}
		pin := wiring.PinnedProducerSnapshot{Model: name, RunID: storedMember.RunID, SnapshotID: storedMember.SnapshotID}
		context.successfulRuns[name] = producerRun
		pinned[name] = pin
		for _, alias := range memberAliases {
			context.successfulRuns[alias] = producerRun
			pinned[alias] = pin
		}
	}

	var maxAge *time.Duration
	if request.MaxObservationAge != "" {
		parsed, err := time.ParseDuration(request.MaxObservationAge)
		if err != nil {
			return nil, nil, fmt.Errorf("decode approved max observation age: %w", err)
		}
		maxAge = &parsed
	}
	schemas, err := inference.Shared(wsPolicy.SchemaSearchPaths...)
	if err != nil {
		return nil, nil, fmt.Errorf("load binding schemas: %w", err)
	}
	for _, name := range graph.Ordered {
		template := graph.Models[name].Template
		var model *systemmodel.SystemModel
		var evidence []wiring.BindingEvidence
		if len(template.Bindings) == 0 {
			model, err = template.Elaborate(map[string]any{})
		} else {
			var elaboration *wiring.Elaboration
			elaboration, err = (wiring.Resolver{Catalog: db, Schemas: schemas}).Elaborate(template, wiring.ResolveRequest{
				Workspace: effectiveWorkspaceName(), MaxObservationAge: maxAge,
				PinnedProducerSnapshots: pinned,
			})
			if err == nil {
				model, evidence = elaboration.Model, elaboration.Evidence
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("rebuild approved member %q: %w", name, err)
		}
		storedMember := storedMembers[name]
		context.successfulModels[name] = model
		context.snapshotIDs[name] = storedMember.SnapshotID
		context.modelDirs[name] = template.Origin.LoadDir
		context.bindingEvidence[name] = evidence
	}

	refs, configured := wsPolicy.SecretsWritablePolicy()
	sealedMembers := make([]wiring.SealedMember, 0, len(graph.Ordered))
	for _, name := range graph.Ordered {
		sealedMembers = append(sealedMembers, wiring.SealedMember{
			Model: context.successfulModels[name], Aliases: aliases[name], RunID: storedMembers[name].RunID,
		})
	}
	resolved, err := wiring.ResolveSealedSources(sealedMembers, wiring.SealedPolicy{
		WritableRefs: refs, WritableConfigured: configured,
	})
	if err != nil {
		return nil, nil, err
	}
	for _, member := range resolved {
		context.successfulModels[member.Model.Name] = member.Model
		context.sealedEvidence[member.Model.Name] = member.Evidence
	}
	return graph, context, nil
}

func continueMutatingRunSet(db *database.CatalogDB, graph *acute.RunSetPlan, report *acute.RunSetReport, context *runSetExecutionContext, request runSetMutationRequest) error {
	mutationPlan, prepared, err := buildRunSetMutationPlan(db, graph, report, context, request, false)
	if err != nil {
		report.Status = database.RunStatusFailed
		_ = saveRunSetReport(db, report)
		return err
	}
	digest, err := mutationPlan.CanonicalDigest()
	if err != nil {
		return err
	}
	report.PlanDigest = digest
	for index := range report.Members {
		if member := prepared[report.Members[index].Model]; member != nil {
			report.Members[index].MutationRequired = member.required
		}
	}

	if request.RequireApproval {
		requestJSON, err := json.Marshal(request)
		if err != nil {
			return err
		}
		planJSON, err := json.Marshal(mutationPlan)
		if err != nil {
			return err
		}
		report.Status = "pending-approval"
		report.ApprovalID = report.RunSetID
		report.ApprovalStatus = "pending"
		members, err := pendingMutationMembers(report, prepared)
		if err != nil {
			return err
		}
		reportJSON, err := json.Marshal(report)
		if err != nil {
			return err
		}
		if err := db.SavePendingRunSetApproval(database.PendingRunSetApproval{
			RunSetID: report.RunSetID, PlanDigest: digest,
			Request: requestJSON, Plan: planJSON, Report: reportJSON,
			SnapshotIDs: mutationPlanSnapshotIDs(prepared), Members: members,
		}); err != nil {
			return err
		}
		printRunSetApprovalReview(mutationPlan, context)
		if err := printRunSetReport(report); err != nil {
			return err
		}
		if !jsonOutput {
			fmt.Printf("approval pending: pudl run-set resume %s | pudl run-set reject %s\n", report.RunSetID, report.RunSetID)
		}
		return nil
	}

	if err := prepareMutationMemberRuns(db, report, prepared, false); err != nil {
		return err
	}
	report.ApprovalStatus = "not-required"
	return executePreparedMutationPlan(db, report, mutationPlan, prepared)
}

func printRunSetApprovalReview(plan *acute.RunSetMutationPlan, context *runSetExecutionContext) {
	if jsonOutput || plan == nil || context == nil {
		return
	}
	fmt.Printf("exact mutation plan %s\n", plan.RunSetID)
	fmt.Printf("  digest: %s\n", mustMutationPlanDigest(plan))
	for _, modelName := range plan.Ordered {
		model := context.successfulModels[modelName]
		if model == nil {
			continue
		}
		printOutputs := func(phase string, outputs map[string]systemmodel.SealedOutput) {
			names := make([]string, 0, len(outputs))
			for name := range outputs {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				output := outputs[name]
				fmt.Printf("  write: %s.%s.%s -> %s (%s)\n", modelName, phase, name, output.Ref, output.StoreMode)
			}
		}
		if model.Converge != nil {
			printOutputs("converge", model.Converge.SealedOutputs)
		}
	}
}

func mustMutationPlanDigest(plan *acute.RunSetMutationPlan) string {
	digest, err := plan.CanonicalDigest()
	if err != nil {
		return "<invalid>"
	}
	return digest
}

func pendingMutationMembers(report *acute.RunSetReport, prepared map[string]*preparedMutationMember) ([]database.PendingRunSetMember, error) {
	var members []database.PendingRunSetMember
	for index := range report.Members {
		member := prepared[report.Members[index].Model]
		if member == nil || !member.required {
			continue
		}
		member.report.Mode = "converge"
		member.report.CompletionStatus = database.RunStatusRunning
		member.report.PendingApproval = true
		member.report.ApprovalStatus = "pending"
		report.Members[index].Result = database.RunStatusRunning
		payload, err := json.Marshal(member.report)
		if err != nil {
			return nil, err
		}
		members = append(members, database.PendingRunSetMember{
			RunID: member.runID, Model: member.model.Name, Report: payload,
		})
	}
	return members, nil
}

func buildRunSetMutationPlan(db *database.CatalogDB, graph *acute.RunSetPlan, report *acute.RunSetReport, context *runSetExecutionContext, request runSetMutationRequest, revalidate bool) (*acute.RunSetMutationPlan, map[string]*preparedMutationMember, error) {
	prepared := make(map[string]*preparedMutationMember, len(graph.Ordered))
	members := make([]acute.RunSetMutationMemberPlan, 0, len(graph.Ordered))
	for _, name := range graph.Ordered {
		model := context.successfulModels[name]
		if model == nil {
			return nil, nil, fmt.Errorf("mutation planning lacks elaborated model %q", name)
		}
		run := context.successfulRuns[name]
		record, err := db.GetRunReport(run.RunID)
		if err != nil {
			return nil, nil, err
		}
		if record == nil {
			return nil, nil, fmt.Errorf("mutation planning lacks member report for %q", name)
		}
		var memberReport RunReport
		if err := json.Unmarshal(record.Report, &memberReport); err != nil {
			return nil, nil, fmt.Errorf("decode member report for %q: %w", name, err)
		}
		if revalidate {
			memberReport.Bindings = append([]wiring.BindingEvidence(nil), context.bindingEvidence[name]...)
			memberReport.SealedBindings = append([]wiring.SealedBindingEvidence(nil), context.sealedEvidence[name]...)
		}
		required := false
		var reconcile *reconcileWorkspace
		if model.Convergent() {
			if revalidate {
				memberRoot, err := mutationMuRoot(request.MuRoot, context.modelDirs[name])
				if err != nil {
					return nil, nil, fmt.Errorf("revalidate mutation for %q: %w", name, err)
				}
				reconcile, err = setupReconcileWorkspace(
					&runCatalog{dir: effectivePudlDir(), opened: true, db: db},
					runMuRunnerFactory(), model, memberRoot, context.modelDirs[name], run.RunID, true,
				)
				if err != nil {
					return nil, nil, fmt.Errorf("revalidate mutation for %q: %w", name, err)
				}
				drift, observeErr := reconcile.observeDrift()
				if observeErr != nil {
					reconcile.Cleanup()
					return nil, nil, fmt.Errorf("revalidate mutation for %q: %w", name, observeErr)
				}
				memberReport.Drift = &drift
			}
			if memberReport.Drift == nil {
				if reconcile != nil {
					reconcile.Cleanup()
				}
				return nil, nil, fmt.Errorf("mutating member %q produced no read-only drift decision", name)
			}
			required = !memberReport.Drift.Clean
		}

		member := &preparedMutationMember{
			model: model, report: &memberReport, runID: run.RunID,
			snapshotID: context.snapshotIDs[name], modelDir: context.modelDirs[name], required: required,
		}
		var muPlan []byte
		if required {
			member.muRoot, err = mutationMuRoot(request.MuRoot, member.modelDir)
			if err != nil {
				return nil, nil, fmt.Errorf("plan mutation for %q: %w", name, err)
			}
			if reconcile == nil {
				reconcile, err = setupReconcileWorkspace(
					&runCatalog{dir: effectivePudlDir(), opened: true, db: db},
					runMuRunnerFactory(), model, member.muRoot, member.modelDir, member.runID, true,
				)
				if err != nil {
					return nil, nil, fmt.Errorf("plan mutation for %q: %w", name, err)
				}
			}
			planned, planErr := reconcile.planConverge()
			stagingDir := reconcile.Dir
			reconcile.Cleanup()
			reconcile = nil
			if planErr != nil {
				return nil, nil, fmt.Errorf("plan mutation for %q: %w", name, redactSealedError(planErr, model))
			}
			rawPlan := []byte(planned)
			if err := annotateSealedActionClaims(&memberReport, rawPlan, model); err != nil {
				return nil, nil, fmt.Errorf("record sealed action routing for %q: %w", name, err)
			}
			muPlan, err = canonicalMutationPlan(rawPlan, stagingDir)
			if err != nil {
				return nil, nil, fmt.Errorf("canonicalize mu plan for %q: %w", name, err)
			}
			member.report = &memberReport
		}
		if reconcile != nil {
			reconcile.Cleanup()
		}
		planMember, err := acute.NewRunSetMutationMemberPlan(
			model, member.runID, member.snapshotID, memberReport.Bindings, muPlan, required,
		)
		if err != nil {
			return nil, nil, err
		}
		member.expectedMuPlanSHA256 = planMember.MuPlanSHA256
		prepared[name] = member
		members = append(members, planMember)
	}

	refs, configured := wsPolicy.SecretsWritablePolicy()
	return &acute.RunSetMutationPlan{
		PlanVersion: 1, RunSetID: report.RunSetID, Mode: "converge",
		Options: acute.RunSetMutationOptions{
			MaxObservationAge: request.MaxObservationAge,
			MaxIterations:     request.MaxIterations, MaxApplies: request.MaxApplies,
		},
		Edges: graph.Edges, Ordered: graph.Ordered, Members: members,
		WritableRefsConfigured:  configured,
		WritableRefFingerprints: acute.FingerprintWritableRefs(refs),
	}, prepared, nil
}

func canonicalMutationPlan(plan []byte, stagingDir string) ([]byte, error) {
	document, _, err := validateMuMutationPlan(plan)
	if err != nil {
		return nil, err
	}
	canonical := canonicalPlanValue(document, stagingDir)
	payload, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("encode canonical mu plan: %w", err)
	}
	return payload, nil
}

func canonicalPlanValue(value any, stagingDir string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if key == "action_key" || key == "plan_sha256" {
				continue
			}
			out[canonicalPlanString(key, stagingDir)] = canonicalPlanValue(item, stagingDir)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = canonicalPlanValue(item, stagingDir)
		}
		return out
	case string:
		return canonicalPlanString(typed, stagingDir)
	default:
		return value
	}
}

func validateMuMutationPlan(plan []byte) (any, string, error) {
	var document any
	decoder := json.NewDecoder(strings.NewReader(string(plan)))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, "", fmt.Errorf("decode mu JSON plan: %w", err)
	}
	root, ok := document.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("mu JSON plan must be an object")
	}
	version, ok := root["version"].(json.Number)
	if !ok || version.String() != "2" {
		return nil, "", fmt.Errorf("mu JSON plan version must be exactly 2")
	}
	digest, ok := root["plan_sha256"].(string)
	if !ok || len(digest) != sha256.Size*2 {
		return nil, "", fmt.Errorf("mu JSON plan is missing a valid plan_sha256")
	}
	plugins, exists := root["plugins"].([]any)
	if !exists {
		return nil, "", fmt.Errorf("mu JSON plan v2 is missing plugin identities")
	}
	for index, item := range plugins {
		identity, ok := item.(map[string]any)
		if !ok || stringField(identity, "name") == "" || stringField(identity, "digest") == "" || stringField(identity, "version") == "" {
			return nil, "", fmt.Errorf("mu JSON plan plugin %d lacks immutable name, digest, or version identity", index)
		}
		protocol, ok := identity["protocol_version"].(json.Number)
		if !ok || protocol.String() == "0" {
			return nil, "", fmt.Errorf("mu JSON plan plugin %d lacks protocol identity", index)
		}
		if _, ok := identity["capabilities"].([]any); !ok {
			return nil, "", fmt.Errorf("mu JSON plan plugin %d lacks capability identity", index)
		}
	}
	actions, ok := root["actions"].([]any)
	if !ok {
		return nil, "", fmt.Errorf("mu JSON plan v2 is missing actions")
	}
	for index, item := range actions {
		action, ok := item.(map[string]any)
		if !ok || stringField(action, "id") == "" || stringField(action, "action_key") == "" {
			return nil, "", fmt.Errorf("mu JSON plan action %d lacks id or action_key", index)
		}
		for _, field := range []string{"command", "inputs", "outputs", "depends_on"} {
			if _, exists := action[field]; !exists {
				return nil, "", fmt.Errorf("mu JSON plan action %d lacks required %s field", index, field)
			}
		}
	}

	delete(root, "plan_sha256")
	payload, err := json.Marshal(root)
	if err != nil {
		return nil, "", fmt.Errorf("encode mu JSON plan identity: %w", err)
	}
	actual := sha256.Sum256(payload)
	if hex.EncodeToString(actual[:]) != digest {
		return nil, "", fmt.Errorf("mu JSON plan_sha256 does not match its plan content")
	}
	root["plan_sha256"] = digest
	return document, digest, nil
}

func stringField(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func canonicalPlanString(value, stagingDir string) string {
	if stagingDir == "" {
		return value
	}
	return strings.ReplaceAll(value, stagingDir, "<pudl-reconcile-workspace>")
}

type plannedMuActions struct {
	Actions []struct {
		ID            string            `json:"id"`
		SealedInputs  map[string]string `json:"sealed_inputs,omitempty"`
		SealedOutputs map[string]string `json:"sealed_outputs,omitempty"`
	} `json:"actions"`
}

func annotateSealedActionClaims(report *RunReport, plan []byte, model *systemmodel.SystemModel) error {
	var document plannedMuActions
	if err := json.Unmarshal(plan, &document); err != nil {
		return fmt.Errorf("decode mu JSON plan: %w", err)
	}
	for _, action := range document.Actions {
		for _, ref := range modelSealedReferences(model) {
			if strings.Contains(action.ID, ref) {
				return fmt.Errorf("mu plan action id contains a sealed provider reference")
			}
		}
	}
	if len(report.SealedBindings) == 0 {
		return nil
	}
	for index := range report.SealedBindings {
		evidence := &report.SealedBindings[index]
		switch evidence.Direction {
		case "input":
			if evidence.ConsumerPhase != "converge" {
				continue
			}
			for _, action := range document.Actions {
				if _, claimed := action.SealedInputs[evidence.Input]; claimed {
					evidence.ClaimingActionIDs = append(evidence.ClaimingActionIDs, action.ID)
				}
			}
			sort.Strings(evidence.ClaimingActionIDs)
			if len(evidence.ClaimingActionIDs) == 0 {
				return fmt.Errorf("converge sealed input %q has no claiming action", evidence.Input)
			}
		case "output":
			if evidence.ProducerPhase != "converge" {
				continue
			}
			for _, action := range document.Actions {
				if _, claimed := action.SealedOutputs[evidence.Output]; !claimed {
					continue
				}
				if evidence.ProducingActionID != "" {
					return fmt.Errorf("converge sealed output %q has multiple producing actions", evidence.Output)
				}
				evidence.ProducingActionID = action.ID
			}
			if evidence.ProducingActionID == "" {
				return fmt.Errorf("converge sealed output %q has no producing action", evidence.Output)
			}
		}
	}
	return nil
}

func mutationMuRoot(requested, modelDir string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	root, err := findMuRoot(modelDir)
	if err != nil {
		return "", err
	}
	return root, nil
}

func prepareMutationMemberRuns(db *database.CatalogDB, report *acute.RunSetReport, prepared map[string]*preparedMutationMember, pendingApproval bool) error {
	for index := range report.Members {
		member := prepared[report.Members[index].Model]
		if member == nil || !member.required {
			continue
		}
		if err := db.PrepareRunMutation(member.runID); err != nil {
			return err
		}
		member.report.Mode = "converge"
		member.report.CompletionStatus = database.RunStatusRunning
		member.report.PendingApproval = pendingApproval
		if pendingApproval {
			member.report.ApprovalStatus = "pending"
		}
		report.Members[index].Result = database.RunStatusRunning
		if err := saveMemberRunReport(db, member.report); err != nil {
			return err
		}
	}
	return nil
}

func executePreparedMutationPlan(db *database.CatalogDB, report *acute.RunSetReport, plan *acute.RunSetMutationPlan, prepared map[string]*preparedMutationMember) error {
	cat := &runCatalog{dir: effectivePudlDir(), opened: true, db: db}
	results := make(map[string]string, len(report.Members))
	for _, member := range report.Members {
		results[member.Model] = member.Result
	}
	mutationFailed := false
	for _, name := range plan.Ordered {
		member := prepared[name]
		if member == nil || !member.required {
			continue
		}
		if mutationFailed {
			status := database.RunStatusCancelled
			note := "cancelled after an earlier mutating member failed"
			if blocker := firstNonSuccessfulPrerequisite(name, plan.Edges, results); blocker != "" {
				status = database.RunStatusBlocked
				note = fmt.Sprintf("blocked by unsuccessful prerequisite %q", blocker)
			}
			if err := finishUnstartedMutationMember(db, report, member, status, note); err != nil {
				return err
			}
			results[name] = status
			continue
		}

		member.report.PendingApproval = false
		member.report.ApprovalStatus = report.ApprovalStatus
		budget := resolveApplyBudget(cat, name, runFlags{
			converge: true, maxIters: plan.Options.MaxIterations, maxApplies: plan.Options.MaxApplies,
		}, !jsonOutput)
		convergeReport, runErr := runConvergeLoopExact(
			cat, runMuRunnerFactory(), member.model, member.muRoot, member.modelDir,
			member.runID, plan.Options.MaxIterations, false, budget, member.expectedMuPlanSHA256,
		)
		member.report.Converge = convergeReport
		applyRunError(member.report, runErr)
		status := database.RunStatusSucceeded
		if runErr != nil || convergeReport == nil || convergeReport.Outcome != string(outcomeClean) {
			status = database.RunStatusFailed
			mutationFailed = true
			if runErr == nil {
				runErr = fmt.Errorf("convergence ended %s", convergeReport.Outcome)
				applyRunError(member.report, runErr)
			}
		} else {
			member.report.CompletionStatus = status
		}
		if convergeReport != nil && len(convergeReport.MutationReceipts) > 0 {
			advanceSealedLifecycle(member.report)
		}
		verdict := runVerdict(member.report, runFlags{converge: true})
		conclusion := database.RunConclusion{CompletionStatus: status, Verdict: verdict}
		if convergeReport != nil {
			conclusion.Outcome = convergeReport.Outcome
			conclusion.NeedsVerification = convergeReport.NeedsVerification
		}
		if runErr != nil {
			conclusion.Note = runErr.Error()
		}
		if err := db.FinishRun(member.runID, conclusion); err != nil {
			return err
		}
		if err := saveMemberRunReport(db, member.report); err != nil {
			return err
		}
		updateRunSetMember(report, name, status, errorString(runErr))
		results[name] = status
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

func finishUnstartedMutationMember(db *database.CatalogDB, report *acute.RunSetReport, member *preparedMutationMember, status, note string) error {
	member.report.PendingApproval = false
	member.report.CompletionStatus = status
	member.report.OK = false
	member.report.Error = note
	if err := db.FinishRun(member.runID, database.RunConclusion{CompletionStatus: status, Note: note}); err != nil {
		return err
	}
	if err := saveMemberRunReport(db, member.report); err != nil {
		return err
	}
	updateRunSetMember(report, member.model.Name, status, note)
	return saveRunSetReport(db, report)
}

func updateRunSetMember(report *acute.RunSetReport, model, status, note string) {
	for index := range report.Members {
		if report.Members[index].Model == model {
			report.Members[index].Result = status
			report.Members[index].Error = note
			return
		}
	}
}

func advanceSealedLifecycle(report *RunReport) {
	for index := range report.SealedBindings {
		if report.SealedBindings[index].Direction == "output" {
			report.SealedBindings[index].LifecycleStatus = "stored"
		} else {
			report.SealedBindings[index].LifecycleStatus = "delivered"
		}
	}
}

func saveMemberRunReport(db *database.CatalogDB, report *RunReport) error {
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return db.SaveRunReport(report.RunID, report.Model, payload)
}

func retainMutationPlanSnapshots(db *database.CatalogDB, prepared map[string]*preparedMutationMember, retain bool) error {
	for _, snapshotID := range mutationPlanSnapshotIDs(prepared) {
		if err := db.RetainObserveSnapshot(snapshotID, retain); err != nil {
			return fmt.Errorf("retain mutation-plan snapshot %q: %w", snapshotID, err)
		}
	}
	return nil
}

func mutationPlanSnapshotIDs(prepared map[string]*preparedMutationMember) []string {
	seen := map[string]struct{}{}
	for _, member := range prepared {
		if member.report.Populate != nil && member.report.Populate.SnapshotID != "" {
			seen[member.report.Populate.SnapshotID] = struct{}{}
		}
		for _, binding := range member.report.Bindings {
			if binding.SnapshotID != "" {
				seen[binding.SnapshotID] = struct{}{}
			}
		}
	}
	snapshotIDs := make([]string, 0, len(seen))
	for snapshotID := range seen {
		snapshotIDs = append(snapshotIDs, snapshotID)
	}
	sort.Strings(snapshotIDs)
	return snapshotIDs
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
