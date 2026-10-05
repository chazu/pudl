package acute

import (
	"fmt"
	"strings"
	"time"
)

// Drift reasons. Every reason other than none is drift: a desired resource that
// cannot be compared is not evidence that it matches.
const (
	// DriftMissing: no observed record carries the desired resource's identity.
	DriftMissing = "missing"
	// DriftChanged: the observed record exists but a desired field differs.
	DriftChanged = "changed"
	// DriftUnidentifiable: the desired record carries no identity to match on,
	// so it cannot be compared with anything.
	DriftUnidentifiable = "unidentifiable"
	// DriftAmbiguous: several differing observed records carry the desired
	// resource's identity, so there is no single record to compare against.
	DriftAmbiguous = "ambiguous"
)

// FieldDiff is one desired field the observed record does not satisfy. Path is
// a dotted path into the record ("spec.replicas", "ports[0].port").
type FieldDiff struct {
	Path     string `json:"path"`
	Expected any    `json:"expected,omitempty"`
	Observed any    `json:"observed,omitempty"`
	// Missing: the desired field is absent from the observed record.
	Missing bool `json:"missing,omitempty"`
	// Unexpected: a nested observed field the desired value does not declare.
	// Nested values compare exactly, so an extra key below the top level drifts;
	// extra top-level fields do not (ensure-present semantics).
	Unexpected bool `json:"unexpected,omitempty"`
}

// ResourceDrift is a single drifted resource and why.
type ResourceDrift struct {
	Resource string `json:"resource"` // "Kind/name"
	Reason   string `json:"reason"`   // missing | changed | unidentifiable | ambiguous
	// Fields lists every unsatisfied desired field, in path order. Empty for a
	// missing resource, and for a differential observer that reports only a
	// textual diff.
	Fields []FieldDiff `json:"fields,omitempty"`
	// ObservedAt is when the compared observed record was recorded, when known.
	ObservedAt *time.Time `json:"observed_at,omitempty"`
	// Diff is a one-line human summary, kept for compatibility with consumers of
	// earlier reports.
	Diff string `json:"diff,omitempty"`
}

// ModelDriftResult is the instance-level drift verdict over an observation:
// clean iff every desired resource exists and matches.
type ModelDriftResult struct {
	Clean   bool            `json:"clean"`
	Drifted []ResourceDrift `json:"drifted,omitempty"`

	// Verified reports whether this verdict came from a fresh observation of the
	// live system. A catalog replay leaves it false: the records it compares
	// against may predate the last apply, so a clean replay must not promote
	// resources to clean or write a clean model status. False is the default so
	// that a path which forgets to claim verification stays untrusted rather
	// than silently authoritative.
	Verified bool `json:"verified"`

	// ObservationID is the catalog entry recording the observation this verdict
	// came from, so a `clean` claim can be traced to stored evidence rather than
	// resting on a value that existed only in memory. Empty for a catalog replay,
	// which observed nothing, and for a dry run, which persists nothing.
	ObservationID string `json:"observation_id,omitempty"`

	// SnapshotID is the observation snapshot the verdict compared against, when
	// the comparison was scoped to one.
	SnapshotID string `json:"snapshot_id,omitempty"`

	// ObservedAt is when the compared observation was taken, when known.
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

// StampObservedAt records when the observation behind a verdict was taken, on
// the verdict and on every finding that does not already carry its own time.
func (r *ModelDriftResult) StampObservedAt(at time.Time) {
	r.ObservedAt = &at
	for i := range r.Drifted {
		if r.Drifted[i].ObservedAt == nil {
			r.Drifted[i].ObservedAt = &at
		}
	}
}

// summarizeFields renders field differences as the one-line Diff summary.
func summarizeFields(fields []FieldDiff) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, f.String())
	}
	return strings.Join(parts, "; ")
}

// String renders one field difference in the compact summary form used by Diff
// ("default_branch: release → want main"). Values print plainly, as earlier
// reports did, unless the plain forms would read as equal — 1 and "1" — in which
// case both print as JSON so the difference stays visible.
func (f FieldDiff) String() string {
	switch {
	case f.Missing:
		return fmt.Sprintf("%s missing (want %v)", f.Path, f.Expected)
	case f.Unexpected:
		return fmt.Sprintf("%s: unexpected %v", f.Path, f.Observed)
	}
	observed, expected := fmt.Sprintf("%v", f.Observed), fmt.Sprintf("%v", f.Expected)
	if observed == expected {
		observed, expected = FormatValue(f.Observed), FormatValue(f.Expected)
	}
	return fmt.Sprintf("%s: %s → want %s", f.Path, observed, expected)
}

// Detail renders one field difference with both values as JSON, for the
// per-field lines of the human report.
func (f FieldDiff) Detail() string {
	switch {
	case f.Missing:
		return fmt.Sprintf("%s: expected %s, observed (absent)", f.Path, FormatValue(f.Expected))
	case f.Unexpected:
		return fmt.Sprintf("%s: expected (absent), observed %s", f.Path, FormatValue(f.Observed))
	default:
		return fmt.Sprintf("%s: expected %s, observed %s", f.Path, FormatValue(f.Expected), FormatValue(f.Observed))
	}
}
