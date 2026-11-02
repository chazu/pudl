package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/chazu/pudl/internal/acute"
	"strings"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/secretscan"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/wiring"
)

// RunReport is the structured result of a `pudl run`, collected across phases and
// rendered as markdown (human, default) or JSON (--json, machine/agent/CI). Both
// outputs carry the same data — the design is agent-native (README:36,445).
type RunReport struct {
	Summary          *acute.VerdictSummary    `json:"summary,omitempty"`
	ReportVersion    int                      `json:"report_version"`
	RunSetID         string                   `json:"run_set_id,omitempty"`
	RunID            string                   `json:"run_id"`
	Model            string                   `json:"model"`
	Mode             string                   `json:"mode"` // observe-only | observe-only (inventory) | checks-only
	CompletionStatus string                   `json:"completion_status"`
	Populate         *PopulateReport          `json:"populate,omitempty"`
	Drift            *ModelDriftResult        `json:"drift,omitempty"`
	Checks           []CheckResult            `json:"checks,omitempty"`
	OK               bool                     `json:"ok"` // overall: no phase error, no fail-severity check failed
	Error            string                   `json:"error,omitempty"`
	Bindings         []wiring.BindingEvidence `json:"bindings,omitempty"`
	BindingIssues    []wiring.BindingIssue    `json:"unresolved_bindings,omitempty"`
}

// applyRunError copies a phase error into the report before it is rendered or
// persisted. Reports are persisted before RunE returns, so relying only on the
// deferred named-return finalizer would lose the error details.
func applyRunError(report *RunReport, err error) {
	if report == nil || err == nil {
		return
	}
	report.OK = false
	report.Error = err.Error()
	report.CompletionStatus = failureStatus(err)
}

func resolutionDiagnosticReport(template *systemmodel.ModelTemplate, err error) *RunReport {
	mode := "observe-only"
	model := ""
	if template != nil {
		model = template.Name
	}
	return &RunReport{
		ReportVersion: 2, Model: model, Mode: mode,
		CompletionStatus: database.RunStatusFailed, OK: false, Error: errorString(err),
		BindingIssues: resolutionBindingIssues(template, err),
	}
}

// PopulateReport summarizes an inventory populate.
type PopulateReport struct {
	RequestedSchema  string `json:"requested_schema,omitempty"`
	SchemaPolicy     string `json:"schema_policy,omitempty"`
	SchemaMismatches int    `json:"schema_mismatches,omitempty"`
	Target           string `json:"target"`
	Records          int    `json:"records"`
	SnapshotID       string `json:"snapshot_id,omitempty"`
	// SecretWarnings are likely secrets outside sensitive-field coverage, by
	// record, path and kind (never the value); stored data is unchanged.
	SecretWarnings      []secretscan.Warning `json:"secret_warnings,omitempty"`
	SecretWarningsTotal int                  `json:"secret_warnings_total,omitempty"`
}

// render emits the report as JSON when machine output is requested, else markdown.
func (r *RunReport) render(asJSON bool) (string, error) {
	if r.ReportVersion == 0 {
		r.ReportVersion = 2
	}
	if r.ReportVersion >= 2 && r.Summary == nil {
		r.Summary = summarizeRun(r, nil)
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
	if r.Summary != nil {
		s := r.Summary
		fmt.Fprintf(&b, "Execution: %s | conformity: %s | checks: %s | verification: %s | scope: %s\n\n", s.Execution, s.Conformity, s.Checks, s.Verification, s.Scope)
	}
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

	if r.Populate != nil {
		fmt.Fprintf(&b, "\n## populate\n- target: %s\n- records: %d\n", r.Populate.Target, r.Populate.Records)
		if r.Populate.SnapshotID != "" {
			fmt.Fprintf(&b, "- snapshot_id: %s\n", r.Populate.SnapshotID)
		}
	}
	if r.Drift != nil || len(r.Checks) > 0 {
		fmt.Fprintf(&b, "\n## findings\n")
	}
	if r.Drift != nil {
		fmt.Fprintf(&b, "\n### drift\n")
		r.Drift.WriteMarkdown(&b)
	}
	if len(r.Checks) > 0 {
		fmt.Fprintf(&b, "\n### checks\n")
		for _, c := range r.Checks {
			if c.Outcome == "unknown" || c.Outcome == "error" {
				fmt.Fprintf(&b, "  - ? %s [%s]: %s\n", c.Name, c.Severity, c.Outcome)
				for _, d := range c.Diagnostics {
					fmt.Fprintf(&b, "    - %s: %s\n", d.Code, d.Message)
				}
				continue
			}
			if c.Passed {
				fmt.Fprintf(&b, "  - ✓ %s (%s)\n", c.Name, c.Severity)
			} else {
				fmt.Fprintf(&b, "  - ✗ %s [%s] — %d match(es): %s\n", c.Name, c.Severity, c.Count, c.Message)
			}
			writeCheckFindings(&b, c)
		}
	}
	return b.String()
}
