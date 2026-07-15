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
	RunID    string            `json:"run_id"`
	Model    string            `json:"model"`
	Mode     string            `json:"mode"` // observe-only | converge | dry-run
	Populate *PopulateReport   `json:"populate,omitempty"`
	Drift    *ModelDriftResult `json:"drift,omitempty"`
	Checks   []CheckResult     `json:"checks,omitempty"`
	Converge *ConvergeReport   `json:"converge,omitempty"`
	OK       bool              `json:"ok"` // overall: no fail-severity check failed, converge not failed
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
			if c.Passed {
				fmt.Fprintf(&b, "  - ✓ %s (%s)\n", c.Name, c.Severity)
			} else {
				fmt.Fprintf(&b, "  - ✗ %s [%s] — %d match(es): %s\n", c.Name, c.Severity, c.Count, c.Message)
			}
		}
	}
	if r.Converge != nil {
		fmt.Fprintf(&b, "\n## converge\n- iterations: %d\n- outcome: %s\n", r.Converge.Iterations, r.Converge.Outcome)
	}
	return b.String()
}
