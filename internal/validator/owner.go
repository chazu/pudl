package validator

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/ownership"
)

// decodeOwner strictly decodes `_pudl.owner`: how a resource's owner is
// derived. A present but invalid block (not concrete, an unknown key, a
// wildcard path, an empty owner) records OwnerError, which
// CheckModuleIntegrity reports as a schema load error and which disables
// projection for the schema: a typo must neither vanish silently nor turn
// every resource unowned.
func decodeOwner(pudl cue.Value, meta *SchemaMetadata) {
	v := pudl.LookupPath(cue.ParsePath("owner"))
	if !v.Exists() {
		return
	}
	spec, err := parseOwner(v)
	if err != nil {
		meta.OwnerError = fmt.Sprintf("owner: %v", err)
		return
	}
	meta.Owner = spec
}

func parseOwner(v cue.Value) (*ownership.Spec, error) {
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return nil, err
	}
	raw, err := v.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return ownership.Parse(raw)
}
