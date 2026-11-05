package checkreport

import (
	"fmt"
	"io"
	"time"

	"github.com/chazu/pudl/internal/waiver"
)

const noEarlierReport = "no comparable earlier report"

// WriteChanges writes a classified check's findings for humans: the counts by
// state against the earlier reports, how the findings spread over projects
// and owners, then each finding with when it was first seen. It writes nothing
// when the check has no findings now or before, unless a note explains why it
// was not compared.
func WriteChanges(w io.Writer, c CheckResult) {
	writeChanges(w, c, nil)
}

// writeChanges is WriteChanges leaving out findings whose resource the model's
// shared section lists.
func writeChanges(w io.Writer, c CheckResult, shared map[string]bool) {
	ch := c.Changes
	if ch == nil {
		return
	}
	total := ch.New + ch.Persisting + ch.Resolved + ch.Unknown
	if total == 0 {
		if ch.Note != "" && ch.Note != noEarlierReport {
			fmt.Fprintf(w, "    - not compared with %s: %s\n", ch.Since, ch.Note)
		}
		return
	}
	since := noEarlierReport
	if ch.Since != "" {
		since = "since " + ch.Since
		if ch.SinceAt != nil {
			since += " (" + ch.SinceAt.UTC().Format(time.RFC3339) + ")"
		}
	}
	waived := ""
	if ch.Waived > 0 {
		waived = fmt.Sprintf(" (%d waived)", ch.Waived)
	}
	fmt.Fprintf(w, "    - %s: %d new, %d persisting, %d resolved, %d unknown%s\n", since, ch.New, ch.Persisting, ch.Resolved, ch.Unknown, waived)
	if ch.Note != "" && ch.Note != noEarlierReport {
		fmt.Fprintf(w, "    - note: %s\n", ch.Note)
	}
	if c.Expect != "empty" {
		return // one finding per check: the counts say it all
	}
	writeFindingSummary(w, c.Findings)
	writeFindingList(w, c.Findings, shared, witnessWaivers(c.Witnesses), true)
	if c.FindingsTruncated {
		fmt.Fprintf(w, "    - findings truncated to %d; counts cover recorded findings only\n", FindingLimit)
	}
}

// witnessWaivers maps each witness's finding key to the waiver it matched,
// active or expired.
func witnessWaivers(witnesses []Witness) map[string]*waiver.Ref {
	waivers := map[string]*waiver.Ref{}
	for _, w := range witnesses {
		if w.Waiver != nil {
			waivers[w.Key] = w.Waiver
		}
	}
	return waivers
}
