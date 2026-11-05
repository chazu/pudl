// Package checkreport is the recorded result of `pudl check`: one model's
// report (ModelReport) and its checks, their verdict summaries, and their
// human rendering. The check's aggregate report is acute.RunSetReport; this
// package summarizes, renders and reads it back.
package checkreport

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/evidence"
	"github.com/chazu/pudl/internal/populate"
	"github.com/chazu/pudl/internal/projection"
	"github.com/chazu/pudl/internal/secretscan"
	"github.com/chazu/pudl/internal/waiver"
)

// ModelReportVersion is the version a model report is recorded with.
const ModelReportVersion = 2

// ModelReport is the structured result of one model within a `pudl check`,
// collected across phases and rendered as markdown (human, default) or JSON
// (--json, machine/agent/CI). Both outputs carry the same data.
type ModelReport struct {
	Summary          *acute.VerdictSummary   `json:"summary,omitempty"`
	ReportVersion    int                     `json:"report_version"`
	RunSetID         string                  `json:"run_set_id,omitempty"`
	RunID            string                  `json:"run_id"`
	Model            string                  `json:"model"`
	Mode             string                  `json:"mode"` // observe-only | observe-only (inventory) | checks-only
	CompletionStatus string                  `json:"completion_status"`
	Populate         *Populate               `json:"populate,omitempty"`
	Drift            *acute.ModelDriftResult `json:"drift,omitempty"`
	Checks           []CheckResult           `json:"checks,omitempty"`
	// Waivers are the model's declared waivers after its checks: active,
	// expired or stale, with how many findings each matched.
	Waivers []waiver.Outcome `json:"waivers,omitempty"`
	OK      bool             `json:"ok"` // overall: no phase error, no fail-severity check failed
	Error   string           `json:"error,omitempty"`
}

// Populate summarizes a model's populate phase.
type Populate struct {
	RequestedSchema  string `json:"requested_schema,omitempty"`
	SchemaPolicy     string `json:"schema_policy,omitempty"`
	SchemaMismatches int    `json:"schema_mismatches,omitempty"`
	Target           string `json:"target"`
	// Records counts the records observed in the snapshot (what `pudl
	// snapshot show` reports); NewRecords counts those stored as new catalog
	// entries. A re-run over unchanged resources has NewRecords 0.
	Records    int    `json:"records"`
	NewRecords int    `json:"new_records"`
	SnapshotID string `json:"snapshot_id,omitempty"`
	// SecretWarnings are likely secrets outside sensitive-field coverage, by
	// record, path and kind (never the value); stored data is unchanged.
	SecretWarnings      []secretscan.Warning `json:"secret_warnings,omitempty"`
	SecretWarningsTotal int                  `json:"secret_warnings_total,omitempty"`
	// Inputs are the inputs `each` enumerated, recorded for provenance.
	Inputs []string `json:"inputs,omitempty"`
	// Runs counts the runs attempted; FailedInputs are those that failed.
	// Their resources are unknown: the observation is incomplete.
	Runs         int                    `json:"runs,omitempty"`
	FailedInputs []populate.FailedInput `json:"failed_inputs,omitempty"`
	// EmptyInputs are runs whose failure matched populate.empty_when: they
	// were observed with no records and do not make the observation
	// incomplete. Listed so a declared-empty input stays visible.
	EmptyInputs []populate.EmptyInput `json:"empty_inputs,omitempty"`
	// Fallback is set when populate.fallback answered instead of the
	// populate's own runs; Inputs, Runs and the input lists are then the
	// fallback's.
	Fallback *populate.FallbackUse `json:"fallback,omitempty"`
}

// CheckResult is the outcome of one model check (a Datalog relation evaluated
// over the catalog, asserted empty/nonempty). Count is the number of result
// rows.
type CheckResult struct {
	Evidence           []evidence.Reference    `json:"evidence,omitempty"`
	Outcome            string                  `json:"outcome,omitempty"`
	Diagnostics        []projection.Diagnostic `json:"diagnostics,omitempty"`
	Expect             string                  `json:"expect,omitempty"`
	Witnesses          []Witness               `json:"witnesses,omitempty"`
	WitnessesTruncated bool                    `json:"witnesses_truncated,omitempty"`
	// Findings identify every failing match (bounded by FindingLimit) and,
	// once classified, earlier ones that no longer match; Changes counts them
	// by state against the model's comparable earlier reports.
	Findings          []Finding `json:"findings,omitempty"`
	FindingsTruncated bool      `json:"findings_truncated,omitempty"`
	Changes           *Changes  `json:"changes,omitempty"`
	Name              string    `json:"name"`
	Query             string    `json:"query"`
	Severity          string    `json:"severity"`
	Count             int       `json:"count"`
	Scope             string    `json:"scope"`
	Passed            bool      `json:"passed"`
	Message           string    `json:"message,omitempty"`
	// Waived counts the findings an active waiver excuses. When it equals
	// Count the outcome is waived.
	Waived int `json:"waived,omitempty"`
}

// OutcomeWaived is the outcome of a failed check whose every finding an
// active waiver excuses. It is not a pass: Passed stays false.
const OutcomeWaived = "waived"

// Uncertain reports whether the check could not establish a verdict.
func (c CheckResult) Uncertain() bool { return c.Outcome == "unknown" || c.Outcome == "error" }

// IsWaived reports whether the check failed only on waived findings.
func (c CheckResult) IsWaived() bool { return c.Outcome == OutcomeWaived }

// Failing reports whether the check did not pass and is not waived.
func (c CheckResult) Failing() bool { return !c.Passed && !c.IsWaived() }

// Render emits the report as indented JSON when asJSON, else as markdown. A
// version-2 report without a summary gets one computed from its own fields.
func (r *ModelReport) Render(asJSON bool) (string, error) {
	if r.ReportVersion == 0 {
		r.ReportVersion = ModelReportVersion
	}
	if r.ReportVersion >= 2 && r.Summary == nil {
		r.Summary = Summarize(r, nil)
	}
	if asJSON {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	}
	return r.Markdown(), nil
}

// Markdown renders the human report.
func (r *ModelReport) Markdown() string {
	var b strings.Builder
	if r.Summary != nil {
		s := r.Summary
		fmt.Fprintf(&b, "Execution: %s | conformity: %s | checks: %s | verification: %s | scope: %s\n\n", s.Execution, s.Conformity, s.Checks, s.Verification, s.Scope)
	}
	fmt.Fprintf(&b, "# model: %s\n\n", r.Model)
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

	if r.Populate != nil {
		r.Populate.writeMarkdown(&b)
	}
	if r.Drift != nil || len(r.Checks) > 0 {
		fmt.Fprintf(&b, "\n## findings\n")
	}
	if r.Drift != nil {
		fmt.Fprintf(&b, "\n### drift\n")
		r.Drift.WriteMarkdown(&b)
	}
	shared := sharedResources(r.Checks)
	if len(r.Checks) > 0 {
		fmt.Fprintf(&b, "\n### checks\n")
		for _, c := range r.Checks {
			if c.Uncertain() {
				fmt.Fprintf(&b, "  - ? %s [%s]: %s\n", c.Name, c.Severity, c.Outcome)
				for _, d := range c.Diagnostics {
					fmt.Fprintf(&b, "    - %s: %s\n", d.Code, d.Message)
				}
				writeChanges(&b, c, shared)
				continue
			}
			switch {
			case c.Passed:
				fmt.Fprintf(&b, "  - ✓ %s (%s)\n", c.Name, c.Severity)
			case c.IsWaived():
				fmt.Fprintf(&b, "  - ~ %s [%s] waived — %d match(es), all waived: %s\n", c.Name, c.Severity, c.Count, c.Message)
			case c.Waived > 0:
				fmt.Fprintf(&b, "  - ✗ %s [%s] — %d match(es), %d waived: %s\n", c.Name, c.Severity, c.Count, c.Waived, c.Message)
			default:
				fmt.Fprintf(&b, "  - ✗ %s [%s] — %d match(es): %s\n", c.Name, c.Severity, c.Count, c.Message)
			}
			if c.Changes == nil {
				writeFindings(&b, c, shared)
				continue
			}
			if !c.Passed && c.Expect == "nonempty" {
				writeNoEvidence(&b, c)
			}
			writeChanges(&b, c, shared)
		}
	}
	writeSharedResources(&b, r.Checks, shared)
	writeWaivers(&b, r.Waivers)
	return b.String()
}
