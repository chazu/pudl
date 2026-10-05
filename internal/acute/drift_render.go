package acute

import (
	"fmt"
	"io"
	"time"
)

// WriteMarkdown renders the drift verdict as the human report's drift section
// body. It reads the same fields the JSON report serializes, so the two outputs
// cannot disagree about a finding: every resource with its reason, every field
// difference, when the compared observation was taken, and the evidence behind
// the verdict.
func (r ModelDriftResult) WriteMarkdown(w io.Writer) {
	if r.Clean {
		fmt.Fprintf(w, "- ∅ (clean — all desired resources exist and match)\n")
	} else {
		fmt.Fprintf(w, "- %d drifted resource(s):\n", len(r.Drifted))
		for _, d := range r.Drifted {
			if d.Diff != "" {
				fmt.Fprintf(w, "  - ~ %s (%s): %s\n", d.Resource, d.Reason, d.Diff)
			} else {
				fmt.Fprintf(w, "  - ~ %s (%s)\n", d.Resource, d.Reason)
			}
			if len(d.Fields) == 0 && d.Previous != nil {
				fmt.Fprintf(w, "    - previous: %s\n", d.Previous.Detail())
			}
			for _, f := range d.Fields {
				fmt.Fprintf(w, "    - %s\n", f.Detail())
			}
			if d.ObservedAt != nil && (r.ObservedAt == nil || !d.ObservedAt.Equal(*r.ObservedAt)) {
				fmt.Fprintf(w, "    - observed_at: %s\n", formatTime(*d.ObservedAt))
			}
		}
	}
	fmt.Fprintf(w, "- verified: %t\n", r.Verified)
	if r.ObservedAt != nil {
		fmt.Fprintf(w, "- observed_at: %s\n", formatTime(*r.ObservedAt))
	}
	if r.SnapshotID != "" {
		fmt.Fprintf(w, "- snapshot_id: %s\n", r.SnapshotID)
	}
	if r.ObservationID != "" {
		fmt.Fprintf(w, "- observation_id: %s\n", r.ObservationID)
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
