package inference

import (
	"fmt"

	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// claimBonus orders claimed schemas ahead of schemas that merely share field
// names; the ordering is cosmetic, since claimedOnly restricts the cascade.
const claimBonus = 1.0

// claims reports whether data matches the `@pudl(match)` fields of schema or one of
// its ancestors, so a specialization of a claimed type is claimed too.
func claims(schema string, metadata map[string]validator.SchemaMetadata, graph *InheritanceGraph, data interface{}) bool {
	chain := []string{schema}
	if graph != nil {
		chain = graph.GetCascadeChain(schema)
	}
	for _, name := range chain {
		if metadata[name].Claims(data) {
			return true
		}
	}
	return false
}

// claimedOnly returns the claimed candidates, or nil when none are claimed.
// A record that claims a type is assigned a claimed type or rejected; it never
// falls back to a generic schema.
func claimedOnly(candidates []CandidateScore) []CandidateScore {
	var claimed []CandidateScore
	for _, c := range candidates {
		if c.Claimed {
			claimed = append(claimed, c)
		}
	}
	return claimed
}

// ClaimError is returned when a record claims a type (by `@pudl(match)`) but
// satisfies none of the claimed schemas.
type ClaimError struct {
	Schema string
	Err    error
}

func (e *ClaimError) Error() string {
	return fmt.Sprintf("record matches %s (@pudl(match)) but does not satisfy it: %v", schemaname.Normalize(e.Schema), e.Err)
}

func (e *ClaimError) Unwrap() error { return e.Err }

// Claims reports whether data claims any loaded type by `@pudl(match)`.
// Callers that route records without inference (observe ingest) use it to
// send claimed records through Infer, which assigns or rejects them.
func (si *SchemaInferrer) Claims(data interface{}) bool {
	si.mu.RLock()
	defer si.mu.RUnlock()
	for _, meta := range si.metadata {
		if !meta.IsPolicy() && meta.Claims(data) {
			return true
		}
	}
	return false
}
