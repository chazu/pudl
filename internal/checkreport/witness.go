package checkreport

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/chazu/pudl/internal/datalog"
)

// WitnessLimit bounds how many witnesses a failed check records; counts
// include every match.
const WitnessLimit = 20

// witnessValueLimit bounds one witness argument's encoded size.
const witnessValueLimit = 4096

// Witness contains only projected query-head arguments, never a recursive
// proof or hidden body row.
type Witness struct {
	Resource    string         `json:"resource,omitempty"`
	Arguments   map[string]any `json:"arguments"`
	EvidenceIDs []string       `json:"evidence_ids,omitempty"`
	// Owner is the owner of the resource or catalog entry the witness
	// references (see AttributeOwners); "" when none was resolved.
	Owner string `json:"owner"`
}

// Witnesses selects the deterministic, bounded witnesses of an `expect:
// empty` check from its result tuples. truncated reports that more matched
// than were kept.
func Witnesses(tuples []datalog.Tuple, expect string) (witnesses []Witness, truncated bool) {
	if expect != "empty" || len(tuples) == 0 {
		return nil, false
	}
	type candidate struct {
		witness Witness
		key     string
	}
	candidates := make([]candidate, 0, len(tuples))
	for _, tuple := range tuples {
		args := make(map[string]any, len(tuple.Args))
		for key, value := range tuple.Args {
			encoded, err := json.Marshal(value)
			if err != nil || len(encoded) > witnessValueLimit {
				value = "<omitted: value exceeds witness limit>"
			}
			args[key] = value
		}
		witness := Witness{Arguments: args}
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
		encoded, _ := json.Marshal(args)
		candidates = append(candidates, candidate{witness: witness, key: string(encoded)})
	}
	sort.Slice(candidates, func(i, j int) bool {
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
	return witnesses, len(candidates) > count
}

// WriteFindings writes what a failed check found: its witnesses, or why a
// nonempty check has none.
func WriteFindings(w io.Writer, result CheckResult) {
	if result.Passed {
		return
	}
	if result.Expect == "nonempty" && !result.Passed {
		fmt.Fprintf(w, "    - no matching evidence for relation %s in %s scope (expected nonempty)\n", result.Query, result.Scope)
	}
	writeWitnesses(w, result.Witnesses)
	if result.WitnessesTruncated {
		fmt.Fprintf(w, "    - witnesses truncated to %d; counts include all matches\n", len(result.Witnesses))
	}
}
