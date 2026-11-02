package checkreport

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/waiver"
)

// WitnessLimit bounds how many witnesses a failed check records; counts
// include every match.
const WitnessLimit = 20

// witnessValueLimit bounds one witness argument's encoded size.
const witnessValueLimit = 4096

// Witness contains only projected query-head arguments, never a recursive
// proof or hidden body row.
type Witness struct {
	// Key is the match's finding identity (FindingKey of its full arguments).
	Key         string         `json:"key,omitempty"`
	Resource    string         `json:"resource,omitempty"`
	Arguments   map[string]any `json:"arguments"`
	EvidenceIDs []string       `json:"evidence_ids,omitempty"`
	// Waiver is the model waiver this finding matched: active when it is
	// waived, expired when an expired waiver no longer excuses it.
	Waiver *waiver.Ref `json:"waiver,omitempty"`
}

// Waived reports whether an active waiver excuses this finding.
func (w Witness) Waived() bool { return w.Waiver != nil && w.Waiver.State == waiver.Active }

// Witnesses selects the deterministic, bounded witnesses of an `expect:
// empty` check from its result tuples. truncated reports that more matched
// than were kept.
func Witnesses(tuples []datalog.Tuple, expect string) (witnesses []Witness, truncated bool) {
	witnesses, truncated, _, _ = WaivedWitnesses(tuples, expect, nil)
	return witnesses, truncated
}

// MatchFunc decides which waiver, if any, a finding with these witness
// arguments matched (see waiver.Matcher.Match).
type MatchFunc func(args map[string]any) (*waiver.Ref, error)

// WaivedWitnesses is Witnesses with each finding matched against waivers.
// Every tuple is matched, not only the kept ones; waived counts the findings
// an active waiver excuses. Unwaived witnesses come first, so a truncated
// list shows what still needs action. A nil match waives nothing.
func WaivedWitnesses(tuples []datalog.Tuple, expect string, match MatchFunc) (witnesses []Witness, truncated bool, waived int, err error) {
	if expect != "empty" || len(tuples) == 0 {
		return nil, false, 0, nil
	}
	type candidate struct {
		witness Witness
		key     string
	}
	candidates := make([]candidate, 0, len(tuples))
	for _, tuple := range tuples {
		witness := newWitness(tuple)
		if match != nil {
			if witness.Waiver, err = match(tuple.Args); err != nil {
				return nil, false, 0, err
			}
		}
		if witness.Waived() {
			waived++
		}
		encoded, _ := json.Marshal(witness.Arguments)
		candidates = append(candidates, candidate{witness: witness, key: string(encoded)})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if a, b := candidates[i].witness.Waived(), candidates[j].witness.Waived(); a != b {
			return b
		}
		return candidates[i].key < candidates[j].key
	})
	count := len(candidates)
	if count > WitnessLimit {
		count = WitnessLimit
	}
	witnesses = make([]Witness, count)
	for i := range witnesses {
		witnesses[i] = candidates[i].witness
	}
	return witnesses, len(candidates) > count, waived, nil
}

// newWitness projects one result tuple, bounding each argument's size.
func newWitness(tuple datalog.Tuple) Witness {
	args := make(map[string]any, len(tuple.Args))
	for key, value := range tuple.Args {
		encoded, err := json.Marshal(value)
		if err != nil || len(encoded) > witnessValueLimit {
			value = "<omitted: value exceeds witness limit>"
		}
		args[key] = value
	}
	witness := Witness{Key: FindingKey(tuple.Args), Arguments: args}
	for _, key := range []string{"resource", "target", "name"} {
		if value, ok := args[key].(string); ok {
			witness.Resource = value
			break
		}
	}
	for _, key := range []string{"entry_id", "evidence_id", "observation_id", "snapshot_id"} {
		if value, ok := args[key].(string); ok && value != "" {
			witness.EvidenceIDs = append(witness.EvidenceIDs, value)
		}
	}
	sort.Strings(witness.EvidenceIDs)
	return witness
}

// WriteFindings writes what a failed or waived check found: its witnesses,
// each with the waiver it matched, or why a nonempty check has none.
func WriteFindings(w io.Writer, result CheckResult) {
	if result.Passed {
		return
	}
	if result.Expect == "nonempty" {
		writeNoEvidence(w, result)
	}
	for _, witness := range result.Witnesses {
		args, _ := json.Marshal(witness.Arguments)
		fmt.Fprintf(w, "    - %s%s\n", args, waiverNote(witness.Waiver))
	}
	if result.WitnessesTruncated {
		fmt.Fprintf(w, "    - witnesses truncated to %d; counts include all matches\n", len(result.Witnesses))
	}
}

// waiverNote describes the waiver a witness matched.
func waiverNote(ref *waiver.Ref) string {
	if ref == nil {
		return ""
	}
	if ref.State == waiver.Active {
		return fmt.Sprintf(" — waived by waivers[%d] (%s, until %s): %s", ref.Index, ref.Owner, ref.Expires, ref.Reason)
	}
	return fmt.Sprintf(" — waivers[%d] (%s) expired %s; this finding counts again", ref.Index, ref.Owner, ref.Expires)
}

// writeNoEvidence explains a failing `expect: nonempty` check, which has no
// witnesses.
func writeNoEvidence(w io.Writer, result CheckResult) {
	fmt.Fprintf(w, "    - no matching evidence for relation %s in %s scope (expected nonempty)\n", result.Query, result.Scope)
}
