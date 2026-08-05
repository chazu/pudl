package systemmodel

import (
	"fmt"
	"sort"

	"cuelang.org/go/cue"
)

func bindingProducerNames(value cue.Value, bindings map[string]ValueBinding) ([]string, error) {
	set := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		set[binding.Source.Model] = struct{}{}
	}
	for _, phase := range []string{"populate", "converge"} {
		inputs, err := decodeSealedInputs(value, phase+".sealed_inputs")
		if err != nil {
			return nil, err
		}
		for _, input := range inputs {
			if input.Source != nil {
				set[input.Source.Model] = struct{}{}
			}
		}
	}
	producers := make([]string, 0, len(set))
	for producer := range set {
		producers = append(producers, producer)
	}
	sort.Strings(producers)
	return producers, nil
}

func validateSealedDeclarations(value cue.Value) error {
	for _, phase := range []string{"populate", "converge"} {
		phaseValue := value.LookupPath(cue.ParsePath(phase))
		if !phaseValue.Exists() {
			continue
		}
		if err := validateSealedMap(phaseValue, phase, "sealed_inputs"); err != nil {
			return err
		}
	}
	converge := value.LookupPath(cue.ParsePath("converge"))
	if converge.Exists() {
		return validateSealedMap(converge, "converge", "sealed_outputs")
	}
	return nil
}

func validateSealedMap(phaseValue cue.Value, phase, field string) error {
	declarations := phaseValue.LookupPath(cue.ParsePath(field))
	if !declarations.Exists() {
		return nil
	}
	iter, err := declarations.Fields()
	if err != nil {
		return fmt.Errorf("%s.%s must be a map: %w", phase, field, err)
	}
	for iter.Next() {
		name := iter.Label()
		class, err := bindingClass(iter.Value())
		if err != nil {
			return fmt.Errorf("%s.%s.%s: %w", phase, field, name, err)
		}
		if class != BindingSealed {
			return fmt.Errorf("%s.%s.%s must declare @pudl(binding=sealed)", phase, field, name)
		}
	}
	return nil
}

type sealedInputWire struct {
	Ref          string        `json:"ref,omitempty"`
	Source       *SealedSource `json:"source,omitempty"`
	DeliveryMode string        `json:"delivery_mode"`
}

type sealedOutputWire struct {
	Ref       string `json:"ref"`
	StoreMode string `json:"store_mode"`
}

func decodeSealedInputs(value cue.Value, path string) (map[string]SealedInput, error) {
	declarations := value.LookupPath(cue.ParsePath(path))
	if !declarations.Exists() {
		return nil, nil
	}
	wires := map[string]sealedInputWire{}
	if err := declarations.Decode(&wires); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	out := make(map[string]SealedInput, len(wires))
	for name, wire := range wires {
		out[name] = SealedInput{Ref: wire.Ref, Source: wire.Source, DeliveryMode: wire.DeliveryMode}
	}
	return out, nil
}

func decodeSealedOutputs(value cue.Value, path string) (map[string]SealedOutput, error) {
	declarations := value.LookupPath(cue.ParsePath(path))
	if !declarations.Exists() {
		return nil, nil
	}
	wires := map[string]sealedOutputWire{}
	if err := declarations.Decode(&wires); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	out := make(map[string]SealedOutput, len(wires))
	for name, wire := range wires {
		out[name] = SealedOutput{Ref: wire.Ref, StoreMode: wire.StoreMode}
	}
	return out, nil
}

func decodeSealedDeclarations(value cue.Value, model *SystemModel) error {
	var err error
	model.Populate.SealedInputs, err = decodeSealedInputs(value, "populate.sealed_inputs")
	if err != nil {
		return err
	}
	if model.Converge != nil {
		model.Converge.SealedInputs, err = decodeSealedInputs(value, "converge.sealed_inputs")
		if err != nil {
			return err
		}
		model.Converge.SealedOutputs, err = decodeSealedOutputs(value, "converge.sealed_outputs")
		if err != nil {
			return err
		}
	}
	return nil
}
