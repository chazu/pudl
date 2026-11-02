package checkreport

import (
	"fmt"
	"strings"
)

// writeMarkdown renders the populate section: what was recorded, the inputs
// `each` enumerated, the empty inputs, and each failed input with its exit
// status and the last line of its stderr (JSON carries the whole tail).
func (p *Populate) writeMarkdown(b *strings.Builder) {
	// The populate target is an internal catalog key; JSON carries it.
	fmt.Fprintf(b, "\n## populate\n- records: %d (%d new)\n", p.Records, p.NewRecords)
	if p.SnapshotID != "" {
		fmt.Fprintf(b, "- snapshot_id: %s\n", p.SnapshotID)
	}
	if p.Inputs != nil {
		fmt.Fprintf(b, "- inputs (%d): %s\n", len(p.Inputs), strings.Join(p.Inputs, ", "))
	}
	p.writeEmptyInputs(b)
	if len(p.FailedInputs) == 0 {
		return
	}
	fmt.Fprintf(b, "- failed inputs: %d of %d runs (their resources are unknown; the observation is incomplete)\n", len(p.FailedInputs), p.Runs)
	for _, f := range p.FailedInputs {
		fmt.Fprintf(b, "  - %s: exit %d: %s\n", f.Label(), f.ExitStatus, f.Error)
		if f.Item != "" {
			fmt.Fprintf(b, "    argv: %s\n", strings.Join(f.Argv, " "))
		}
		if line := lastLine(f.Stderr); line != "" {
			fmt.Fprintf(b, "    stderr: %s\n", line)
		}
	}
}

// writeEmptyInputs lists the runs that matched empty_when: observed with no
// records, so they are not failures, but kept visible.
func (p *Populate) writeEmptyInputs(b *strings.Builder) {
	if len(p.EmptyInputs) == 0 {
		return
	}
	fmt.Fprintf(b, "- empty inputs: %d of %d runs (matched empty_when; observed with no records)\n", len(p.EmptyInputs), p.Runs)
	for _, e := range p.EmptyInputs {
		fmt.Fprintf(b, "  - %s: exit %d: matched %q\n", e.Label(), e.ExitStatus, e.Pattern)
		if e.Item != "" {
			fmt.Fprintf(b, "    argv: %s\n", strings.Join(e.Argv, " "))
		}
		if e.Reason != "" {
			fmt.Fprintf(b, "    stderr: %s\n", e.Reason)
		}
	}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
