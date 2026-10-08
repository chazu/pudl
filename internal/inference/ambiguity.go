package inference

import (
	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/schemaname"
)

// maxAlternativeChecks bounds the extra unifications spent looking for a tie.
const maxAlternativeChecks = 2

// tiedAlternatives returns candidates after the winner (at index won) that
// scored exactly as high, belong to a different inheritance family, and also
// accept the data. Base/child pairs share a family and tie routinely, so they
// are not ambiguity. Caller holds si.mu.
func (si *SchemaInferrer) tiedAlternatives(candidates []CandidateScore, won int, jsonBytes []byte, dataByCtx map[*cue.Context]cue.Value) []string {
	winner := candidates[won]
	root := si.graph.IdentityRoot(winner.Schema)
	var out []string
	checks := 0
	for _, c := range candidates[won+1:] {
		if c.Score != winner.Score || checks == maxAlternativeChecks {
			break
		}
		if isCatchallSchema(c.Schema) || si.graph.IdentityRoot(c.Schema) == root {
			continue
		}
		schema, ok := si.schemas[c.Schema]
		if !ok {
			continue
		}
		checks++
		if si.unifyError(schema, jsonBytes, dataByCtx) == nil {
			out = append(out, schemaname.Normalize(c.Schema))
		}
	}
	return out
}
