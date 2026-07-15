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
}
