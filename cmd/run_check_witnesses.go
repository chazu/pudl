package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/datalog"
	"github.com/chazu/pudl/internal/systemmodel"
)

const checkWitnessLimit = 20
const checkWitnessValueLimit = 4096

// CheckWitness contains only projected query-head arguments, never a recursive
// proof or hidden body row. Advisory preserves the --only partition.
type CheckWitness struct {
	Resource    string         `json:"resource,omitempty"`
	Arguments   map[string]any `json:"arguments"`
	EvidenceIDs []string       `json:"evidence_ids,omitempty"`
	Advisory    bool           `json:"advisory,omitempty"`
}

func checkWitnesses(tuples []datalog.Tuple, expect string, scope *acute.TupleScope, model *systemmodel.SystemModel) ([]CheckWitness, bool) {
	if expect != "empty" || len(tuples) == 0 {
		return nil, false
	}
	type candidate struct {
		witness CheckWitness
		key     string
	}
	candidates := make([]candidate, 0, len(tuples))
	for _, tuple := range tuples {
		args := make(map[string]any, len(tuple.Args))
		for key, value := range tuple.Args {
			value = redactWitnessValue(value, model)
			encoded, err := json.Marshal(value)
			if err != nil || len(encoded) > checkWitnessValueLimit {
				value = "<omitted: value exceeds witness limit>"
			}
			args[key] = value
		}
		witness := CheckWitness{Arguments: args, Advisory: scope.Restricted() && scope.Advisory(acute.ArgValues(tuple.Args))}
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
		if candidates[i].witness.Advisory != candidates[j].witness.Advisory {
			return !candidates[i].witness.Advisory
		}
		return candidates[i].key < candidates[j].key
	})
	count := len(candidates)
	if count > checkWitnessLimit {
		count = checkWitnessLimit
	}
	witnesses := make([]CheckWitness, count)
	for i := range witnesses {
		witnesses[i] = candidates[i].witness
	}
	return witnesses, len(candidates) > count
}
func redactWitnessValue(value any, model *systemmodel.SystemModel) any {
	switch value := value.(type) {
	case string:
		return redactSealedText(value, model)
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, v := range value {
			out[key] = redactWitnessValue(v, model)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, v := range value {
			out[i] = redactWitnessValue(v, model)
		}
		return out
	default:
		return value
	}
}
func writeCheckFindings(w io.Writer, result CheckResult) {
	if result.Passed && result.AdvisoryCount == 0 {
		return
	}
	if result.Expect == "nonempty" && !result.Passed {
		fmt.Fprintf(w, "    - no matching evidence for relation %s in %s scope (expected nonempty)\n", result.Query, result.Scope)
	}
	for _, witness := range result.Witnesses {
		args, _ := json.Marshal(witness.Arguments)
		advisory := ""
		if witness.Advisory {
			advisory = " (outside --only scope; advisory)"
		}
		fmt.Fprintf(w, "    - %s%s\n", args, advisory)
	}
	if result.WitnessesTruncated {
		fmt.Fprintf(w, "    - witnesses truncated to %d; counts include all matches\n", len(result.Witnesses))
	}
}
