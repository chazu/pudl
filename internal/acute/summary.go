package acute

import "time"

// ObservationEvidence freezes the selection and its age when a check ran.
type ObservationEvidence struct {
	SnapshotID    string    `json:"snapshot_id,omitempty"`
	ObservationID string    `json:"observation_id,omitempty"`
	Scope         string    `json:"scope"`
	Source        string    `json:"source"`
	ObservedAt    time.Time `json:"observed_at"`
	Age           string    `json:"age_at_check"`
	Complete      bool      `json:"complete"`
}

// VerdictSummary keeps execution, expectations and evidence as separate axes.
type VerdictSummary struct {
	Execution  string `json:"execution"`
	Conformity string `json:"conformity"`
	// Checks is not-checked, pass, waived (no failing check, at least one
	// failing only on waived findings), fail or unknown.
	Checks        string `json:"checks"`
	Verification  string `json:"verification"`
	Scope         string `json:"scope"`
	DriftCount    int    `json:"drift_count"`
	FailedChecks  int    `json:"failed_checks"`
	UnknownChecks int    `json:"unknown_checks"`
	// WaivedChecks counts checks whose every finding is waived; they are not
	// FailedChecks.
	WaivedChecks int `json:"waived_checks,omitempty"`
	// NewFindings and ResolvedFindings count check findings against the
	// model's comparable earlier reports.
	NewFindings      int                   `json:"new_findings,omitempty"`
	ResolvedFindings int                   `json:"resolved_findings,omitempty"`
	Evidence         []ObservationEvidence `json:"evidence,omitempty"`
	NextActions      []string              `json:"next_actions,omitempty"`
}
