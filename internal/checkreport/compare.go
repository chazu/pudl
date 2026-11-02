package checkreport

import (
	"fmt"
	"io"

	"github.com/chazu/pudl/internal/database"
)

// Comparison is a stored report's check findings classified against an
// earlier named report, model by model and check by check.
type Comparison struct {
	Report string         `json:"report"`
	Since  string         `json:"since"`
	Models []ModelChanges `json:"models"`
}

// ModelChanges is one model's checks compared between the two reports.
type ModelChanges struct {
	Model      string         `json:"model"`
	RunID      string         `json:"run_id"`
	SinceRunID string         `json:"since_run_id,omitempty"`
	Note       string         `json:"note,omitempty"`
	Checks     []CheckChanges `json:"checks,omitempty"`
}

// CheckChanges is one check's findings classified against the earlier
// report's evaluation of the same check.
type CheckChanges struct {
	Name     string    `json:"name"`
	Severity string    `json:"severity"`
	Outcome  string    `json:"outcome"`
	Expect   string    `json:"expect"`
	Changes  *Changes  `json:"changes"`
	Findings []Finding `json:"findings,omitempty"`
}

// ID is the stored report's ID: its check ID, or a model report's run ID.
func (s *Stored) ID() string {
	if s.Check != nil {
		return s.Check.RunSetID
	}
	return s.RunID
}

// Compare classifies current's failing matches against since's: new when
// since did not record them, resolved when since did and current, being
// complete, no longer sees them. Models pair by name; a check pairs with the
// earlier evaluation of the same check over the same scope.
func Compare(db *database.CatalogDB, current, since *Stored) (*Comparison, error) {
	now, err := storedModelReports(db, current)
	if err != nil {
		return nil, err
	}
	then, err := storedModelReports(db, since)
	if err != nil {
		return nil, err
	}
	earlier := map[string]datedReport{}
	for _, d := range then {
		earlier[d.report.Model] = d
	}
	comparison := &Comparison{Report: current.ID(), Since: since.ID(), Models: []ModelChanges{}}
	for _, d := range now {
		model := ModelChanges{Model: d.report.Model, RunID: d.runID}
		prior, ok := earlier[d.report.Model]
		if !ok {
			model.Note = fmt.Sprintf("model not in report %s", since.ID())
			comparison.Models = append(comparison.Models, model)
			continue
		}
		model.SinceRunID = prior.runID
		for _, c := range d.report.Checks {
			model.Checks = append(model.Checks, compareCheck(d, prior, c))
		}
		comparison.Models = append(comparison.Models, model)
	}
	return comparison, nil
}

func compareCheck(current, prior datedReport, c CheckResult) CheckChanges {
	out := CheckChanges{Name: c.Name, Severity: c.Severity, Outcome: c.Outcome, Expect: c.Expect}
	o := NewObservation(current.runID, observationMode(current.report), current.at, c, current.populated)
	earlier, ok := findCheck(prior.report, c.Name)
	at := prior.at
	notComparable := func(note string) CheckChanges {
		out.Changes = &Changes{Since: prior.runID, SinceAt: &at, Note: note}
		return out
	}
	if !ok {
		return notComparable("check not in the earlier report")
	}
	if earlier.Uncertain() {
		return notComparable("the earlier report reached no verdict for this check")
	}
	e := NewObservation(prior.runID, observationMode(prior.report), prior.at, earlier, prior.populated)
	if !Comparable(o, e) {
		return notComparable("the earlier evaluation is not comparable: different mode, query, expectation or scope")
	}
	out.Findings, out.Changes = Classify(o, []Observation{e})
	return out
}

// WriteText renders the comparison for humans.
func (c *Comparison) WriteText(w io.Writer) {
	fmt.Fprintf(w, "changes in %s since %s\n", c.Report, c.Since)
	for _, m := range c.Models {
		fmt.Fprintf(w, "\n# model: %s (%s", m.Model, m.RunID)
		if m.SinceRunID != "" {
			fmt.Fprintf(w, " since %s", m.SinceRunID)
		}
		fmt.Fprintln(w, ")")
		if m.Note != "" {
			fmt.Fprintf(w, "- %s\n", m.Note)
		}
		for _, check := range m.Checks {
			fmt.Fprintf(w, "  - %s [%s]: %s\n", check.Name, check.Severity, check.Outcome)
			WriteChanges(w, CheckResult{Expect: check.Expect, Findings: check.Findings, Changes: check.Changes})
		}
	}
}
