package checkreport

import (
	"slices"
	"sort"
	"time"

	"github.com/chazu/pudl/internal/checks"
)

// Changes counts a check's findings by state against its comparable earlier
// reports. Since names the newest of those reports; with none, every failing
// match is new.
type Changes struct {
	Since      string     `json:"since,omitempty"`
	SinceAt    *time.Time `json:"since_at,omitempty"`
	New        int        `json:"new"`
	Persisting int        `json:"persisting"`
	Resolved   int        `json:"resolved"`
	Unknown    int        `json:"unknown"`
	Note       string     `json:"note,omitempty"`
}

// Observation is one recorded evaluation of a check, as change
// classification compares it.
type Observation struct {
	RunID string
	Mode  string
	At    time.Time
	Check CheckResult
	// Complete reports that the evaluation establishes absence: it reached a
	// verdict, recorded every failing match, and saw a complete observation of
	// its scope. Only a complete evaluation can resolve a finding.
	Complete bool
}

// NewObservation describes one check of a model report. populateComplete
// says whether the report's own observation was complete, which decides a
// run-scoped check.
func NewObservation(runID, mode string, at time.Time, check CheckResult, populateComplete bool) Observation {
	complete := !check.Uncertain() && recordedCompletely(check)
	switch check.Scope {
	case checks.ScopeSnapshots:
		complete = complete && len(check.Evidence) > 0
		for _, e := range check.Evidence {
			complete = complete && e.Complete
		}
	case checks.ScopeRun:
		complete = complete && populateComplete
	}
	// A latest-known check sees every recorded fact; a partial observation
	// keeps earlier facts known, so a match disappears only when facts change.
	return Observation{RunID: runID, Mode: mode, At: at, Check: check, Complete: complete}
}

// recordedCompletely reports whether every failing match was recorded:
// findings when the report has them, else (an older report) its witnesses.
func recordedCompletely(c CheckResult) bool {
	if c.Findings != nil || c.Passed {
		return !c.FindingsTruncated
	}
	return !c.WitnessesTruncated
}

// Comparable reports whether earlier is an evaluation of the same check over
// the same scope as current: same mode, name, query, expectation and check
// scope, and the same observation scopes when both name evidence.
func Comparable(current, earlier Observation) bool {
	a, b := current.Check, earlier.Check
	if current.Mode != earlier.Mode || a.Name != b.Name || a.Query != b.Query || a.Expect != b.Expect || a.Scope != b.Scope {
		return false
	}
	if len(a.Evidence) == 0 || len(b.Evidence) == 0 {
		return true
	}
	return slices.Equal(evidenceScopes(a), evidenceScopes(b))
}

func evidenceScopes(c CheckResult) []string {
	scopes := make([]string, 0, len(c.Evidence))
	for _, e := range c.Evidence {
		scopes = append(scopes, e.Scope)
	}
	sort.Strings(scopes)
	return slices.Compact(scopes)
}

// present is what an evaluation saw failing: its findings, or for an older
// report without them, its witnesses, first seen when it was recorded.
func present(o Observation) []Finding {
	c := o.Check
	if c.Uncertain() || c.Passed {
		return nil
	}
	if c.Findings != nil {
		var out []Finding
		for _, f := range c.Findings {
			if f.State == "" || f.State == FindingNew || f.State == FindingPersisting {
				out = append(out, f)
			}
		}
		return out
	}
	at := o.At
	if c.Expect != "empty" {
		return []Finding{{Key: FindingKey(map[string]any{}), FirstSeen: &at, Arguments: map[string]any{}}}
	}
	out := make([]Finding, 0, len(c.Witnesses))
	for _, w := range c.Witnesses {
		key := w.Key
		if key == "" {
			key = FindingKey(w.Arguments)
		}
		out = append(out, Finding{Key: key, FirstSeen: &at, Arguments: w.Arguments})
	}
	return out
}

// Classify sorts current's failing matches into new and persisting, and the
// earlier ones it no longer sees into resolved or unknown. history holds the
// comparable earlier evaluations that reached a verdict, newest first, back to
// and including the newest complete one: a match any of them saw is not new,
// but only a complete current evaluation resolves one. Each finding keeps the
// earliest first-seen time any of them recorded.
func Classify(current Observation, history []Observation) ([]Finding, *Changes) {
	changes := &Changes{}
	if len(history) > 0 {
		at := history[0].At
		changes.Since, changes.SinceAt = history[0].RunID, &at
	} else {
		changes.Note = "no comparable earlier report"
	}
	seen := map[string]Finding{}
	for _, o := range history {
		for _, f := range present(o) {
			if f.FirstSeen == nil {
				at := o.At
				f.FirstSeen = &at
			}
			prior, ok := seen[f.Key]
			if ok {
				if prior.FirstSeen.Before(*f.FirstSeen) {
					f.FirstSeen = prior.FirstSeen
				}
				if prior.Arguments != nil {
					f.Arguments = prior.Arguments // the newest recorded arguments
				}
			}
			seen[f.Key] = f
		}
	}

	var findings []Finding
	matched := map[string]bool{}
	for _, f := range present(current) {
		matched[f.Key] = true
		if f.FirstSeen == nil {
			at := current.At
			f.FirstSeen = &at
		}
		f.State = FindingNew
		if prior, ok := seen[f.Key]; ok {
			f.State = FindingPersisting
			if prior.FirstSeen.Before(*f.FirstSeen) {
				f.FirstSeen = prior.FirstSeen
			}
			if f.Arguments == nil {
				f.Arguments = prior.Arguments
			}
			changes.Persisting++
		} else {
			changes.New++
		}
		findings = append(findings, f)
	}
	var gone []Finding
	for key, f := range seen {
		if matched[key] {
			continue
		}
		f.State = FindingUnknown
		if current.Complete {
			f.State = FindingResolved
			changes.Resolved++
		} else {
			changes.Unknown++
		}
		gone = append(gone, f)
	}
	sort.Slice(gone, func(i, j int) bool { return gone[i].Key < gone[j].Key })
	if changes.Unknown > 0 {
		changes.Note = incompleteNote(current.Check)
	}
	return append(findings, gone...), changes
}

// incompleteNote explains why an evaluation cannot resolve earlier findings.
func incompleteNote(c CheckResult) string {
	switch {
	case c.Uncertain():
		return "this evaluation reached no verdict; earlier findings are unknown, not resolved"
	case c.FindingsTruncated:
		return "more matches than recorded findings; earlier findings not recorded now are unknown"
	default:
		return "the observation is incomplete; earlier findings not seen now are unknown, not resolved"
	}
}
