package checkreport

import (
	"fmt"
	"io"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/waiver"
)

// SetFindings records a check's findings from its result tuples, matching
// each against the model's waivers. A failed check whose every finding an
// active waiver excuses becomes waived; one finding left unwaived — including
// one whose waiver has expired — keeps it failed.
//
// It also records every failing match as a finding (see MatchFindings),
// marked waived when an active waiver excuses each of its matches.
func (c *CheckResult) SetFindings(tuples []datalog.Tuple, match MatchFunc) error {
	waivedKeys := map[string]bool{}
	recording := match
	if match != nil {
		recording = func(args map[string]any) (*waiver.Ref, error) {
			ref, err := match(args)
			key := FindingKey(args)
			waived := ref != nil && ref.State == waiver.Active
			if prior, seen := waivedKeys[key]; !seen || prior {
				waivedKeys[key] = waived
			}
			return ref, err
		}
	}
	witnesses, truncated, waived, err := WaivedWitnesses(tuples, c.Expect, recording)
	if err != nil {
		return fmt.Errorf("check %s: match waivers: %w", c.Name, err)
	}
	c.Witnesses, c.WitnessesTruncated, c.Waived = witnesses, truncated, waived
	if c.Outcome == "fail" && waived > 0 && waived == c.Count {
		c.Outcome = OutcomeWaived
	}
	c.Findings, c.FindingsTruncated = MatchFindings(tuples, c.Expect, c.Outcome, witnesses, waivedKeys)
	return nil
}

// writeWaivers renders the model's waivers: what each excused, which have
// expired and which matched nothing.
func writeWaivers(w io.Writer, outcomes []waiver.Outcome) {
	if len(outcomes) == 0 {
		return
	}
	fmt.Fprintf(w, "\n### waivers\n")
	for _, o := range outcomes {
		switch o.State {
		case waiver.Expired:
			fmt.Fprintf(w, "  - ⚠ expired waivers[%d] %s (%s, expired %s): %d finding(s) count again\n", o.Index, o.Target(), o.Owner, o.Expires, o.Matches)
		case waiver.Stale:
			fmt.Fprintf(w, "  - stale waivers[%d] %s (%s, until %s): matches nothing\n", o.Index, o.Target(), o.Owner, o.Expires)
		default:
			fmt.Fprintf(w, "  - active waivers[%d] %s (%s, until %s): matches %d finding(s)\n", o.Index, o.Target(), o.Owner, o.Expires, o.Matches)
		}
		fmt.Fprintf(w, "    - reason: %s\n", o.Reason)
	}
}
