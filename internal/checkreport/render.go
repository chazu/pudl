package checkreport

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/systemmodel"
)

// WriteCheck writes a check report for humans: with members, each model's
// full report first (a live check has already printed those as they
// concluded), then the check's outcome and one line per model.
func WriteCheck(w io.Writer, report *acute.RunSetReport, members bool) error {
	if members {
		for _, member := range report.Members {
			if err := writeMember(w, member.Report); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(w, "\ncheck %s: %s\n", report.RunSetID, report.Status)
	if s := report.Summary; s != nil {
		fmt.Fprintf(w, "conformity: %s | checks: %s | verification: %s%s\n", s.Conformity, s.Checks, s.Verification, findingCounts(s))
	}
	for _, member := range report.Members {
		line := fmt.Sprintf("  %s: %s (%s)", member.Model, member.Result, member.RunID)
		if s := member.Summary; s != nil {
			line += fmt.Sprintf(" | conformity: %s | checks: %s | verification: %s | scope: %s%s", s.Conformity, s.Checks, s.Verification, s.Scope, findingCounts(s))
		}
		if member.Error != "" {
			line += ": " + member.Error
		}
		fmt.Fprintln(w, strings.TrimSpace(line))
		if member.Summary != nil {
			for _, e := range member.Summary.Evidence {
				fmt.Fprintf(w, "    evidence: %s%s | age at check: %s | complete: %t\n", e.SnapshotID, e.ObservationID, e.Age, e.Complete)
			}
		}
	}
	return nil
}

// findingCounts is the summary's new and resolved findings, when it has any.
func findingCounts(s *acute.VerdictSummary) string {
	if s.NewFindings == 0 && s.ResolvedFindings == 0 {
		return ""
	}
	return fmt.Sprintf(" | findings: %d new, %d resolved", s.NewFindings, s.ResolvedFindings)
}

// writeMember renders one embedded member report as markdown.
func writeMember(w io.Writer, payload json.RawMessage) error {
	if len(payload) == 0 {
		return nil
	}
	report, err := DecodeModelReport(payload)
	if err != nil {
		return fmt.Errorf("decode member report: %w", err)
	}
	text, err := report.Render(false)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(w, "\n"+text)
	return err
}

// RenderPlan renders the phase plan a live check prints before it runs a
// model. Every model is observe-only.
func RenderPlan(m *systemmodel.SystemModel) string {
	var b strings.Builder
	fmt.Fprintf(&b, "model:    %s\n", m.Name)
	fmt.Fprintf(&b, "populate: %s (%s)\n", m.Populate.Kind(), populateRef(m.Populate))
	fmt.Fprintf(&b, "checks:   %d\n", len(m.Checks))
	fmt.Fprintf(&b, "mode:     %s\n", "observe-only")

	fmt.Fprintln(&b, "\nphases:")
	fmt.Fprintln(&b, "  1. populate -> ingest (Accumulate)")
	fmt.Fprintln(&b, "  2. drift             (Unify)")
	fmt.Fprintln(&b, "  3. checks            (flag)")
	fmt.Fprintln(&b, "  4. report")
	return b.String()
}

// populateRef returns a short description of the populate arm.
func populateRef(p systemmodel.Populate) string {
	if p.Kind() == systemmodel.KindNone {
		return "checks-only"
	}
	return fmt.Sprintf("%d runs", len(p.Runs))
}
