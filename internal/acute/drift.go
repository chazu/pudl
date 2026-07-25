package acute

// ResourceDrift is a single drifted resource and why.
type ResourceDrift struct {
	Resource string `json:"resource"` // "Kind/name"
	Reason   string `json:"reason"`   // "missing" | "changed"
	Diff     string `json:"diff,omitempty"`
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
}
