package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

// RunReport is the structured result of a `pudl run`, collected across phases and
// rendered as markdown (human, default) or JSON (--json, machine/agent/CI). Both
// outputs carry the same data — the design is agent-native (README:36,445).
type RunReport struct {
	ReportVersion    int                            `json:"report_version"`
	RunSetID         string                         `json:"run_set_id,omitempty"`
	RunID            string                         `json:"run_id"`
	Model            string                         `json:"model"`
	Mode             string                         `json:"mode"` // observe-only | converge | dry-run
	CompletionStatus string                         `json:"completion_status"`
	Populate         *PopulateReport                `json:"populate,omitempty"`
	Drift            *ModelDriftResult              `json:"drift,omitempty"`
	Checks           []CheckResult                  `json:"checks,omitempty"`
	Converge         *ConvergeReport                `json:"converge,omitempty"`
	OK               bool                           `json:"ok"` // overall: no fail-severity check failed, converge not failed
	Error            string                         `json:"error,omitempty"`
	PendingApproval  bool                           `json:"pending_approval,omitempty"`
	ApprovalStatus   string                         `json:"approval_status,omitempty"`
	Bindings         []wiring.BindingEvidence       `json:"bindings,omitempty"`
	BindingIssues    []wiring.BindingIssue          `json:"unresolved_bindings,omitempty"`
	SealedBindings   []wiring.SealedBindingEvidence `json:"sealed_bindings,omitempty"`
}

// applyRunError copies a phase error into the report before it is rendered or
// persisted. Convergence reports are persisted before RunE returns, so relying
// only on the deferred named-return finalizer would lose the error details.
func applyRunError(report *RunReport, err error) {
	if report == nil || err == nil {
		return
	}
	report.OK = false
	report.Error = err.Error()
	report.CompletionStatus = database.RunStatusFailed
}

func resolutionDiagnosticReport(template *systemmodel.ModelTemplate, flags runFlags, err error) *RunReport {
	mode := "observe-only"
	if flags.dryRun {
		mode = "dry-run"
	} else if flags.converge {
		mode = "converge"
	}
	model := ""
	if template != nil {
		model = template.Name
	}
	return &RunReport{
		ReportVersion: 1, Model: model, Mode: mode,
		CompletionStatus: database.RunStatusFailed, OK: false, Error: errorString(err),
		BindingIssues: resolutionBindingIssues(template, err),
	}
}

// PopulateReport summarizes an inventory populate.
type PopulateReport struct {
	Target     string `json:"target"`
	Records    int    `json:"records"`
	SnapshotID string `json:"snapshot_id,omitempty"`
}

// ConvergeReport summarizes a convergence loop.
type ConvergeReport struct {
	Outcome    string `json:"outcome"` // clean | failed (cap_exhausted) | failed (execute_error) | dry-run …
	Iterations int    `json:"iterations"`

	// NeedsVerification is orthogonal to Outcome: the run mutated the system but
	// cannot prove the result, whichever way the loop ended. It dominates the
	// status verdict.
	NeedsVerification bool              `json:"needs_verification,omitempty"`
	MutationReceipts  []MutationReceipt `json:"mutation_receipts,omitempty"`
}

type MutationReceipt struct {
	Iteration      int    `json:"iteration"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Status         string `json:"status"`
}

// render emits the report as JSON when machine output is requested, else markdown.
func (r *RunReport) render(asJSON bool) (string, error) {
	if r.ReportVersion == 0 {
		r.ReportVersion = 1
	}
	if asJSON {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	}
	return r.markdown(), nil
}

// markdown renders the human report.
func (r *RunReport) markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# run: %s\n\n", r.Model)
	fmt.Fprintf(&b, "- run_id: %s\n", r.RunID)
	fmt.Fprintf(&b, "- mode: %s\n", r.Mode)
	if r.CompletionStatus != "" {
		fmt.Fprintf(&b, "- completion: %s\n", r.CompletionStatus)
	}
	status := "OK"
	if !r.OK {
		status = "FAILED"
	}
	fmt.Fprintf(&b, "- status: %s\n", status)
	if r.ApprovalStatus != "" {
		fmt.Fprintf(&b, "- approval: %s\n", r.ApprovalStatus)
	}
	if r.Error != "" {
		fmt.Fprintf(&b, "- error: %s\n", r.Error)
	}
	if len(r.Bindings) > 0 {
		fmt.Fprintf(&b, "\n## bindings\n")
		for _, binding := range r.Bindings {
			fmt.Fprintf(&b, "- %s <- %s %s%s (%s, snapshot %s, age %s)\n",
				binding.Input, binding.ProducerModel, binding.Schema, binding.Path,
				binding.Selection, binding.SnapshotID, binding.Age)
		}
	}
	if len(r.BindingIssues) > 0 {
		fmt.Fprintf(&b, "\n## unresolved bindings\n")
		for _, issue := range r.BindingIssues {
			fmt.Fprintf(&b, "- %s <- %s %s%s (%s): %s\n",
				issue.Input, issue.ProducerModel, issue.Schema, issue.Path,
				issue.Code, issue.Message)
		}
	}
	if len(r.SealedBindings) > 0 {
		fmt.Fprintf(&b, "\n## sealed bindings\n")
		for _, binding := range r.SealedBindings {
			if binding.Direction == "input" {
				fmt.Fprintf(&b, "- %s.%s <- %s (%s, %s, actions %v)\n", binding.ConsumerPhase, binding.Input, binding.SourceKind, binding.ProviderScheme, binding.LifecycleStatus, binding.ClaimingActionIDs)
				continue
			}
			fmt.Fprintf(&b, "- %s.%s -> %s (%s, %s, action %s)\n", binding.ProducerPhase, binding.Output, binding.StoreMode, binding.ProviderScheme, binding.LifecycleStatus, binding.ProducingActionID)
		}
	}

	if r.Populate != nil {
		fmt.Fprintf(&b, "\n## populate\n- target: %s\n- records: %d\n", r.Populate.Target, r.Populate.Records)
	}
	if r.Drift != nil {
		fmt.Fprintf(&b, "\n## drift\n")
		if r.Drift.Clean {
			fmt.Fprintf(&b, "- ∅ (clean — all desired resources exist and match)\n")
		} else {
			fmt.Fprintf(&b, "- %d drifted resource(s):\n", len(r.Drift.Drifted))
			for _, d := range r.Drift.Drifted {
				if d.Diff != "" {
					fmt.Fprintf(&b, "  - ~ %s (%s): %s\n", d.Resource, d.Reason, d.Diff)
				} else {
					fmt.Fprintf(&b, "  - ~ %s (%s)\n", d.Resource, d.Reason)
				}
			}
		}
	}
	if len(r.Checks) > 0 {
		fmt.Fprintf(&b, "\n## checks\n")
		for _, c := range r.Checks {
			// Advisory matches are rendered even on a pass: a check that only
			// matched outside the run's --only scope did not gate, and saying so is
			// what keeps a silent exit-code drop from looking like a clean check.
			outside := ""
			if c.AdvisoryCount > 0 {
				outside = fmt.Sprintf(" (%d match(es) outside --only scope)", c.AdvisoryCount)
			}
			if c.Passed {
				fmt.Fprintf(&b, "  - ✓ %s (%s)%s\n", c.Name, c.Severity, outside)
			} else {
				fmt.Fprintf(&b, "  - ✗ %s [%s] — %d match(es)%s: %s\n", c.Name, c.Severity, c.Count, outside, c.Message)
			}
		}
	}
	if r.Converge != nil {
		fmt.Fprintf(&b, "\n## converge\n- iterations: %d\n- outcome: %s\n", r.Converge.Iterations, r.Converge.Outcome)
	}
	return b.String()
}
