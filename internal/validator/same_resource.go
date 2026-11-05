package validator

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/sameresource"
)

// decodeSameResource strictly decodes `_pudl.same_resource`: the keys that
// say a record and a record of another schema describe one resource. A
// present but invalid block records SameResourceError, which
// CheckModuleIntegrity reports as a schema load error and which disables
// projection for the schema: a typo must not silently drop every link.
func decodeSameResource(pudl cue.Value, meta *SchemaMetadata) {
	v := pudl.LookupPath(cue.ParsePath("same_resource"))
	if !v.Exists() {
		return
	}
	if err := v.Validate(cue.Concrete(true)); err != nil {
		meta.SameResourceError = fmt.Sprintf("same_resource: %v", err)
		return
	}
	raw, err := v.MarshalJSON()
	if err != nil {
		meta.SameResourceError = fmt.Sprintf("same_resource: %v", err)
		return
	}
	spec, err := sameresource.Parse(raw)
	if err != nil {
		meta.SameResourceError = fmt.Sprintf("same_resource: %v", err)
		return
	}
	meta.SameResource = spec
}
