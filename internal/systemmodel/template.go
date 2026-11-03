package systemmodel

import (
	"fmt"

	"cuelang.org/go/cue"
)

// TemplateOrigin identifies where discovery loaded an authored model. It is
// process-local metadata and is never persisted with a run model.
type TemplateOrigin struct {
	Definition string
	SchemaName string
	LoadDir    string
	PUDLRoot   string
}

// ModelTemplate retains the authored CUE value of a discovered model together
// with the metadata discovery and ordering need before the model is decoded.
type ModelTemplate struct {
	value     cue.Value
	context   *cue.Context
	Name      string
	Origin    TemplateOrigin
	DependsOn []string
}

// NewTemplate validates authoring metadata. The returned template is
// in-memory only.
func NewTemplate(value cue.Value, origin TemplateOrigin) (*ModelTemplate, error) {
	if err := rejectRetiredFields(value); err != nil {
		return nil, err
	}
	if err := value.Validate(cue.Concrete(false)); err != nil {
		return nil, fmt.Errorf("validate template: %w", err)
	}
	name, err := value.LookupPath(cue.ParsePath("name")).String()
	if err != nil || name == "" {
		return nil, fmt.Errorf("template has no concrete name")
	}
	var dependsOn []string
	if declared := value.LookupPath(cue.ParsePath("depends_on")); declared.Exists() {
		if err := declared.Decode(&dependsOn); err != nil {
			return nil, fmt.Errorf("decode depends_on: %w", err)
		}
	}
	return &ModelTemplate{
		value: value, context: value.Context(), Name: name, Origin: origin,
		DependsOn: dependsOn,
	}, nil
}

// Value returns the immutable authored value for process-local inspection.
func (t *ModelTemplate) Value() cue.Value { return t.value }

// Context returns the CUE context that owns Value.
func (t *ModelTemplate) Context() *cue.Context { return t.context }

// Model concretely validates the authored value (run transforms excepted)
// and decodes the runtime projection.
func (t *ModelTemplate) Model() (*SystemModel, error) {
	if err := validateConcrete(t.value); err != nil {
		return nil, fmt.Errorf("model %q: %w", t.Name, err)
	}
	return DecodeValue(t.value)
}
