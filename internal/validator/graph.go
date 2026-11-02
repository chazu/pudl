package validator

import (
	"fmt"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/graphspec"
)

// decodeGraph strictly decodes `_pudl.graph`: how records of the type are
// labelled in the resource graph and which values refer to them. A present
// but invalid block records GraphError, which CheckModuleIntegrity reports
// as a schema load error; graph export then falls back to identity values.
func decodeGraph(pudl cue.Value, meta *SchemaMetadata) {
	v := pudl.LookupPath(cue.ParsePath("graph"))
	if !v.Exists() {
		return
	}
	if err := v.Validate(cue.Concrete(true)); err != nil {
		meta.GraphError = fmt.Sprintf("graph: %v", err)
		return
	}
	raw, err := v.MarshalJSON()
	if err != nil {
		meta.GraphError = fmt.Sprintf("graph: %v", err)
		return
	}
	spec, err := graphspec.Parse(raw)
	if err != nil {
		meta.GraphError = fmt.Sprintf("graph: %v", err)
		return
	}
	meta.Graph = spec
}
