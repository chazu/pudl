package checkreport

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/chazu/pudl/internal/waiver"
)

const noEarlierReport = "no comparable earlier report"

// WriteChanges writes a classified check's findings for humans: the counts by
// state against the earlier reports, then each finding whose arguments were
// recorded, with when it was first seen. It writes nothing when the check has
// no findings now or before, unless a note explains why it was not compared.
func WriteChanges(w io.Writer, c CheckResult) {
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
	// The waiver each current witness matched, active or expired.
	waivers := map[string]*waiver.Ref{}
	for _, w := range c.Witnesses {
		if w.Waiver != nil {
			waivers[w.Key] = w.Waiver
		}
	}
	unlisted := 0
	var lines []ownedLine
	for _, f := range c.Findings {
		if f.Arguments == nil {
			unlisted++
			continue
		}
		args, _ := json.Marshal(f.Arguments)
		first := ""
		if f.FirstSeen != nil {
			first = " (first seen " + f.FirstSeen.UTC().Format(time.RFC3339) + ")"
		}
		if ref := waivers[f.Key]; ref != nil && f.State != FindingResolved && f.State != FindingUnknown {
			first += waiverNote(ref)
		} else if f.Waived {
			first += " — waived"
		}
		lines = append(lines, ownedLine{owner: f.Owner, text: fmt.Sprintf("%s %s%s", f.State, args, first)})
	}
	writeByOwner(w, lines)
	if unlisted > 0 {
		fmt.Fprintf(w, "    - %d more finding(s) without recorded witnesses; --json lists their keys\n", unlisted)
	}
	if c.FindingsTruncated {
		fmt.Fprintf(w, "    - findings truncated to %d; counts cover recorded findings only\n", FindingLimit)
	}
}
