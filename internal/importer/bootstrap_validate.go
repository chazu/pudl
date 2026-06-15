package importer

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// ValidateAgainstBootstrapDef validates data against a named definition in an
// embedded bootstrap schema file, using strict CUE unification.
//
// embedFile is the path under the bootstrap root (e.g. "pudl/nous/nous.cue");
// defName is the definition to validate against (e.g. "#Feedback"). The data is
// JSON-encoded and unified with the definition; a non-nil error means the data
// does not satisfy the schema (missing required field, bad enum value, or — since
// CUE definitions are closed — an unknown field).
//
// This is self-contained: it reads the embedded schema directly and does not
// depend on the on-disk schema repository being populated.
func ValidateAgainstBootstrapDef(embedFile, defName string, data interface{}) error {
	content, err := bootstrapSchemas.ReadFile("bootstrap/" + embedFile)
	if err != nil {
		return fmt.Errorf("bootstrap schema %q not found: %w", embedFile, err)
	}

	ctx := cuecontext.New()
	schema := ctx.CompileBytes(content)
	if schema.Err() != nil {
		return fmt.Errorf("compiling bootstrap schema %q: %w", embedFile, schema.Err())
	}

	def := schema.LookupPath(cue.ParsePath(defName))
	if !def.Exists() {
		return fmt.Errorf("definition %q not found in %q", defName, embedFile)
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding data: %w", err)
	}
	dataValue := ctx.CompileBytes(jsonBytes)
	if dataValue.Err() != nil {
		return fmt.Errorf("encoding data: %w", dataValue.Err())
	}

	// Concrete(true) enforces that required fields are present (an absent
	// required field leaves an incomplete value, which fails the concreteness
	// check). Unify is closed because #Definitions are closed, so unknown fields
	// also fail.
	if err := def.Unify(dataValue).Validate(cue.Concrete(true)); err != nil {
		return err
	}
	return nil
}
