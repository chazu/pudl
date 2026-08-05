package systemmodel

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
)

func decodeInputSlots(value cue.Value) (map[string]cue.Value, error) {
	inputs := value.LookupPath(cue.ParsePath("inputs"))
	if !inputs.Exists() {
		return map[string]cue.Value{}, nil
	}
	iter, err := inputs.Fields(cue.Optional(true))
	if err != nil {
		return nil, fmt.Errorf("inputs must be a flat struct: %w", err)
	}
	out := map[string]cue.Value{}
	for iter.Next() {
		name := iter.Label()
		if iter.IsOptional() {
			return nil, fmt.Errorf("input %q must be required", name)
		}
		slot := iter.Value()
		kind := slot.IncompleteKind()
		if kind&(cue.StructKind|cue.ListKind|cue.BytesKind) != 0 || kind&scalarKinds == 0 {
			return nil, fmt.Errorf("input %q must be a scalar constraint, got %s", name, kind)
		}
		class, err := bindingClass(slot)
		if err != nil {
			return nil, fmt.Errorf("input %q: %w", name, err)
		}
		if class != BindingPlain {
			return nil, fmt.Errorf("input %q must declare @pudl(binding=plain)", name)
		}
		out[name] = slot
	}
	return out, nil
}

const scalarKinds = cue.NullKind | cue.BoolKind | cue.IntKind | cue.FloatKind | cue.StringKind

func decodeBindings(value cue.Value) (map[string]ValueBinding, error) {
	bindings := value.LookupPath(cue.ParsePath("bindings"))
	if !bindings.Exists() {
		return map[string]ValueBinding{}, nil
	}
	out := map[string]ValueBinding{}
	if err := bindings.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode bindings: %w", err)
	}
	for name, binding := range out {
		if err := validateJSONPointer(binding.Path); err != nil {
			return nil, fmt.Errorf("binding %q: %w", name, err)
		}
		if len(binding.Source.Identity) == 0 {
			return nil, fmt.Errorf("binding %q: source identity must not be empty", name)
		}
	}
	return out, nil
}

func validateJSONPointer(path string) error {
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be a non-root RFC 6901 JSON Pointer")
	}
	for _, segment := range strings.Split(path[1:], "/") {
		for i := 0; i < len(segment); i++ {
			if segment[i] != '~' {
				continue
			}
			if i+1 >= len(segment) || (segment[i+1] != '0' && segment[i+1] != '1') {
				return fmt.Errorf("path contains invalid JSON Pointer escape")
			}
			i++
		}
	}
	return nil
}

func bindingClass(value cue.Value) (BindingClass, error) {
	classes := map[BindingClass]bool{}
	if err := collectBindingClasses(value, classes, map[string]bool{}, 0); err != nil {
		return "", err
	}
	if len(classes) > 1 {
		return "", fmt.Errorf("conflicting @pudl binding classifications")
	}
	for class := range classes {
		return class, nil
	}
	return "", nil
}

// ClassifyBinding returns the inherited @pudl(binding=...) classification for
// a CUE field. It is shared by template validation and catalog source-field
// authorization so the two sides cannot implement different inheritance rules.
func ClassifyBinding(value cue.Value) (BindingClass, error) {
	return bindingClass(value)
}

func collectBindingClasses(value cue.Value, classes map[BindingClass]bool, seen map[string]bool, depth int) error {
	if depth > 32 {
		return fmt.Errorf("@pudl classification reference depth exceeded")
	}
	if err := collectLocalBindingClasses(value, classes); err != nil {
		return err
	}
	root, path := value.ReferencePath()
	if root.Exists() && len(path.Selectors()) > 0 {
		key := path.String()
		if !seen[key] {
			seen[key] = true
			referenced := root.LookupPath(path)
			if referenced.Exists() {
				if err := collectBindingClasses(referenced, classes, seen, depth+1); err != nil {
					return err
				}
			}
		}
	}
	op, args := value.Expr()
	if op == cue.AndOp {
		for _, arg := range args {
			if err := collectBindingClasses(arg, classes, seen, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectLocalBindingClasses(value cue.Value, classes map[BindingClass]bool) error {
	for _, attr := range value.Attributes(cue.FieldAttr) {
		if attr.Name() != "pudl" {
			continue
		}
		class, found, err := attr.Lookup(0, "binding")
		if err != nil {
			return fmt.Errorf("invalid @pudl attribute: %w", err)
		}
		if !found {
			continue
		}
		switch BindingClass(class) {
		case BindingPlain, BindingSealed:
			classes[BindingClass(class)] = true
		default:
			return fmt.Errorf("unknown @pudl binding class %q", class)
		}
	}
	return nil
}
