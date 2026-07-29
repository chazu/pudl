package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RunReport is the structured result of a `pudl run`, collected across phases and
// rendered as markdown (human, default) or JSON (--json, machine/agent/CI). Both
// outputs carry the same data — the design is agent-native (README:36,445).
type RunReport struct {
	RunID           string            `json:"run_id"`
	Model           string            `json:"model"`
	Mode            string            `json:"mode"` // observe-only | converge | dry-run
	Populate        *PopulateReport   `json:"populate,omitempty"`
	Drift           *ModelDriftResult `json:"drift,omitempty"`
	Checks          []CheckResult     `json:"checks,omitempty"`
	Converge        *ConvergeReport   `json:"converge,omitempty"`
	OK              bool              `json:"ok"` // overall: no fail-severity check failed, converge not failed
	Error           string            `json:"error,omitempty"`
	PendingApproval bool              `json:"pending_approval,omitempty"`
	ApprovalStatus  string            `json:"approval_status,omitempty"`
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
	NeedsVerification bool `json:"needs_verification,omitempty"`
}

// render emits the report as JSON when machine output is requested, else markdown.
func (r *RunReport) render(asJSON bool) (string, error) {
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
