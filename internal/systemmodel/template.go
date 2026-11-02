package systemmodel

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

// ModelTemplate retains the authored CUE value and its compatible context until
// all plain inputs have been resolved. CUE values are immutable; Elaborate
// always unifies a fresh value and does not mutate the template.
type ModelTemplate struct {
	value            cue.Value
	context          *cue.Context
	Name             string
	Origin           TemplateOrigin
	Inputs           map[string]cue.Value
	Bindings         map[string]ValueBinding
	DependsOn        []string
	BindingProducers []string
}

// NewTemplate validates authoring metadata without requiring bound inputs to be
// concrete. The returned template is in-memory only.
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
	inputs, err := decodeInputSlots(value)
	if err != nil {
		return nil, err
	}
	bindings, err := decodeBindings(value)
	if err != nil {
		return nil, err
	}
	if err := validateInputBindingSet(inputs, bindings); err != nil {
		return nil, err
	}
	var dependsOn []string
	if declared := value.LookupPath(cue.ParsePath("depends_on")); declared.Exists() {
		if err := declared.Decode(&dependsOn); err != nil {
			return nil, fmt.Errorf("decode depends_on: %w", err)
		}
	}
	producers := bindingProducerNames(bindings)
	for _, producer := range producers {
		if producer == name {
			return nil, fmt.Errorf("model %q cannot bind to itself", name)
		}
	}
	return &ModelTemplate{
		value: value, context: value.Context(), Name: name, Origin: origin,
		Inputs: inputs, Bindings: bindings, DependsOn: dependsOn,
		BindingProducers: producers,
	}, nil
}

// Value returns the immutable authored value for process-local inspection.
func (t *ModelTemplate) Value() cue.Value { return t.value }

// Context returns the CUE context that owns Value.
func (t *ModelTemplate) Context() *cue.Context { return t.context }

// Elaborate unifies a complete set of resolved plain scalars into a fresh model
// value, concretely validates it, and decodes the runtime projection.
func (t *ModelTemplate) Elaborate(inputs map[string]any) (*SystemModel, error) {
	if err := validateResolvedInputSet(t.Inputs, inputs); err != nil {
		return nil, err
	}
	injection := t.context.Encode(map[string]any{"inputs": inputs})
	if err := injection.Err(); err != nil {
		return nil, fmt.Errorf("encode resolved inputs: %w", err)
	}
	return t.elaborate(injection)
}

// ElaborateJSON is the lossless catalog-value path. Raw JSON scalars compile in
// the template's CUE context, preserving integers beyond Go's float64 range.
func (t *ModelTemplate) ElaborateJSON(inputs map[string]json.RawMessage) (*SystemModel, error) {
	if err := validateResolvedInputSet(t.Inputs, inputs); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{"inputs": inputs})
	if err != nil {
		return nil, fmt.Errorf("marshal resolved inputs: %w", err)
	}
	injection := t.context.CompileBytes(payload)
	if err := injection.Err(); err != nil {
		return nil, fmt.Errorf("compile resolved inputs: %w", err)
	}
	return t.elaborate(injection)
}

func (t *ModelTemplate) elaborate(injection cue.Value) (*SystemModel, error) {
	resolved := t.value.Unify(injection)
	if err := resolved.Validate(cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("elaborate model %q: %w", t.Name, err)
	}
	return DecodeValue(resolved)
}

// bindingProducerNames lists, sorted and once each, the models the bindings
// read from.
func bindingProducerNames(bindings map[string]ValueBinding) []string {
	set := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		set[binding.Source.Model] = struct{}{}
	}
	producers := make([]string, 0, len(set))
	for producer := range set {
		producers = append(producers, producer)
	}
	sort.Strings(producers)
	return producers
}

func validateResolvedInputSet[T any](slots map[string]cue.Value, values map[string]T) error {
	missing, extra := keyDiff(slots, values)
	if len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("resolved inputs must exactly match declared slots (missing: %s; extra: %s)", joinKeys(missing), joinKeys(extra))
	}
	return nil
}

func validateInputBindingSet(slots map[string]cue.Value, bindings map[string]ValueBinding) error {
	missing, extra := keyDiff(slots, bindings)
	if len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("bindings must exactly match input slots (unbound: %s; extra: %s)", joinKeys(missing), joinKeys(extra))
	}
	return nil
}

func keyDiff[A, B any](left map[string]A, right map[string]B) (missing, extra []string) {
	for key := range left {
		if _, ok := right[key]; !ok {
			missing = append(missing, key)
		}
	}
	for key := range right {
		if _, ok := left[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

func joinKeys(keys []string) string {
	if len(keys) == 0 {
		return "-"
	}
	return strings.Join(keys, ", ")
}
