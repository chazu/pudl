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
	Execution     string                `json:"execution"`
	Conformity    string                `json:"conformity"`
	Checks        string                `json:"checks"`
	Verification  string                `json:"verification"`
	Scope         string                `json:"scope"`
	DriftCount    int                   `json:"drift_count"`
	FailedChecks  int                   `json:"failed_checks"`
	UnknownChecks int                   `json:"unknown_checks"`
	Evidence      []ObservationEvidence `json:"evidence,omitempty"`
	NextActions   []string              `json:"next_actions,omitempty"`
}
