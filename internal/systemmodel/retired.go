package systemmodel

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
)

// RetiredFieldsDoc is where every retired model field's replacement is
// documented.
const RetiredFieldsDoc = "docs/retired-commands.md"

// retiredField is a #SystemModel field that PUDL no longer acts on. The schema
// still accepts its shape so model files that use it compile; the loader then
// rejects the model with an error naming the field, so nothing is silently
// ignored.
type retiredField struct {
	path   string
	reason string
}

var retiredFields = []retiredField{
	{"converge", "convergence was removed; PUDL no longer changes infrastructure"},
	{"populate.sealed_inputs", "sealed values were removed"},
	{"converge.sealed_outputs", "sealed values were removed"},
	{"plugins", "mu plugins were removed"},
	{"populate.plugin", "#PluginObserve was removed; use #CommandObserve runs"},
	{"populate.eweSource", "#EweTarget was removed; use #CommandObserve runs"},
	{"inputs", "value bindings were removed; write the value into the model or read it in the command"},
	{"bindings", "value bindings were removed; order models with depends_on"},
}

// rejectRetiredFields fails when the model uses any retired field. It runs
// before validation so the error names the field instead of a downstream CUE
// failure.
func rejectRetiredFields(value cue.Value) error {
	var used []string
	for _, field := range retiredFields {
		if value.LookupPath(cue.ParsePath(field.path)).Exists() {
			used = append(used, fmt.Sprintf("%s (%s)", field.path, field.reason))
		}
	}
	if len(used) == 0 {
		return nil
	}
	return fmt.Errorf("model uses retired field(s): %s; remove them — see %s for replacements",
		strings.Join(used, "; "), RetiredFieldsDoc)
}
