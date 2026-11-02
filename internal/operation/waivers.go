package operation

import (
	"time"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/chazu/pudl/internal/policyfacts"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/waiver"
)

// newWaiverMatcher prepares the model's waivers as of now, or returns nil
// when it declares none.
func newWaiverMatcher(cat *runCatalog, m *systemmodel.SystemModel, now time.Time) *waiver.Matcher {
	if len(m.Waivers) == 0 {
		return nil
	}
	return waiver.NewMatcher(m.Waivers, now, policyViolations(cat))
}

// findingMatcher matches the findings of one check; nil when nothing can be
// waived.
func findingMatcher(matcher *waiver.Matcher, check string) checkreport.MatchFunc {
	if matcher == nil {
		return nil
	}
	return func(args map[string]any) (*waiver.Ref, error) { return matcher.Match(check, args) }
}

// policyViolations looks up a resource's current violations of a policy in
// the catalog, for policy waivers. They are the latest-known results: an
// evidence-scoped check's view evaluates the same policies over the records
// it selected, which are the latest observation unless the check pinned an
// older snapshot.
func policyViolations(cat *runCatalog) waiver.ViolationLookup {
	return func(resource, policyName string) ([]waiver.Violation, error) {
		db, err := cat.required()
		if err != nil {
			return nil, err
		}
		stored, err := policyfacts.ReadStored(db, resource, policyName)
		if err != nil {
			return nil, err
		}
		var out []waiver.Violation
		for _, s := range stored {
			for _, v := range s.Violations {
				out = append(out, waiver.Violation{Rule: v.Rule, Severity: string(v.Severity), Path: v.Path, Kind: string(v.Kind)})
			}
		}
		return out, nil
	}
}
