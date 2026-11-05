package checkreport

// Resolver identifies the cataloged resource a finding's arguments name.
type Resolver interface {
	// Resolve returns the resource one of args' values identifies, and that
	// value; ok is false when none does.
	Resolve(args map[string]any) (resourceID, value string, ok bool)
	// Describe returns the account or project that owns a resource, its type
	// and its display name ("" when unknown).
	Describe(resourceID string) Description
}

// Description is what a Resolver knows about a resource.
type Description struct {
	Account, Type, Name string
}

// AttributeResources records, on each finding, the cataloged resource its
// arguments name: every finding of this evaluation, recorded arguments or
// not. A finding whose Resource was empty takes the identifying value. Call it
// before AttributeOwners, which then attributes owners by that resource.
func AttributeResources(results []CheckResult, r Resolver) {
	if r == nil {
		return
	}
	for i := range results {
		for j := range results[i].Findings {
			f := &results[i].Findings[j]
			args := f.Arguments
			if f.refs != nil {
				args = f.refs
			}
			id, value, ok := r.Resolve(args)
			if !ok {
				continue
			}
			f.ResourceID = id
			if f.Resource == "" {
				f.Resource = value
			}
			d := r.Describe(id)
			f.Account, f.Type, f.Name = d.Account, d.Type, d.Name
		}
	}
}
