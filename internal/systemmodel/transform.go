package systemmodel

import (
	"bytes"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	cuejson "cuelang.org/go/encoding/json"
)

// Transform is a run's `transform: {in: _, out: ...}`: for each record the
// run prints, out is evaluated with in bound to the record, and out is what
// is stored. out: null drops the record. It stays a CUE value: out refers to
// in, so it cannot be decoded ahead of the records.
type Transform struct {
	value cue.Value
}

var (
	transformIn  = cue.ParsePath("in")
	transformOut = cue.ParsePath("out")
)

// decodeTransforms attaches each run's transform, the populate's and its
// fallback's, read from the CUE value because Decode cannot carry a value
// that depends on in.
func decodeTransforms(inst cue.Value, m *SystemModel) error {
	if err := attachTransforms(inst, "populate.runs", m.Populate.Runs); err != nil {
		return err
	}
	if f := m.Populate.Fallback; f != nil {
		return attachTransforms(inst, "populate.fallback.observe.runs", f.Observe.Runs)
	}
	return nil
}

func attachTransforms(inst cue.Value, path string, runs []CommandRun) error {
	list := inst.LookupPath(cue.ParsePath(path))
	if !list.Exists() {
		return nil
	}
	for i := range runs {
		v := list.LookupPath(cue.MakePath(cue.Index(i), cue.Str("transform")))
		if !v.Exists() {
			continue
		}
		t, err := NewTransform(v)
		if err != nil {
			return fmt.Errorf("%s[%d].transform: %w", path, i, err)
		}
		runs[i].Transform = t
	}
	return nil
}

// NewTransform wraps a CUE `{in: _, out: ...}` value. out is required.
func NewTransform(v cue.Value) (*Transform, error) {
	if err := v.Err(); err != nil {
		return nil, err
	}
	if !declaresOut(v) {
		return nil, fmt.Errorf("out is required")
	}
	return &Transform{value: v}, nil
}

// declaresOut reports whether the transform sets out: the schema declares it
// optional, so only an author's out is a regular field.
func declaresOut(v cue.Value) bool {
	iter, err := v.Fields()
	if err != nil {
		return false
	}
	for iter.Next() {
		if iter.Selector().String() == "out" {
			return true
		}
	}
	return false
}

// Apply evaluates the transform for one record. keep is false when out is
// null. An out that is not concrete, or not an object or null, is an error.
func (t *Transform) Apply(record map[string]any) (out map[string]any, keep bool, err error) {
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, false, err
	}
	expr, err := cuejson.Extract("record", raw)
	if err != nil {
		return nil, false, fmt.Errorf("transform: record: %w", err)
	}
	in := t.value.Context().BuildExpr(expr)
	if err := in.Err(); err != nil {
		return nil, false, fmt.Errorf("transform: record: %w", err)
	}
	res := t.value.FillPath(transformIn, in).LookupPath(transformOut)
	if err := res.Validate(cue.Concrete(true)); err != nil {
		return nil, false, fmt.Errorf("transform: out: %w", err)
	}
	if res.IsNull() {
		return nil, false, nil
	}
	if res.Kind() != cue.StructKind {
		return nil, false, fmt.Errorf("transform: out is %v, not an object or null", res.Kind())
	}
	b, err := res.MarshalJSON()
	if err != nil {
		return nil, false, fmt.Errorf("transform: out: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, false, fmt.Errorf("transform: out: %w", err)
	}
	return out, true, nil
}

// validateConcrete requires the model to be concrete everywhere except in
// run transforms (any `transform` of an element of a `runs` list), where in
// is a placeholder and out depends on it. Transforms are still checked for
// errors.
func validateConcrete(v cue.Value) error {
	if err := v.Validate(cue.Concrete(false)); err != nil {
		return err
	}
	return concreteExceptTransforms(v, false)
}

func concreteExceptTransforms(v cue.Value, isRun bool) error {
	switch v.IncompleteKind() {
	case cue.StructKind:
		iter, err := v.Fields()
		if err != nil {
			return err
		}
		for iter.Next() {
			label, child := iter.Selector().String(), iter.Value()
			if isRun && label == "transform" {
				continue
			}
			if label == "runs" && child.IncompleteKind() == cue.ListKind {
				list, err := child.List()
				if err != nil {
					return err
				}
				for list.Next() {
					if err := concreteExceptTransforms(list.Value(), true); err != nil {
						return err
					}
				}
				continue
			}
			if err := concreteExceptTransforms(child, false); err != nil {
				return err
			}
		}
		return nil
	case cue.ListKind:
		list, err := v.List()
		if err != nil {
			return err
		}
		for list.Next() {
			if err := concreteExceptTransforms(list.Value(), false); err != nil {
				return err
			}
		}
		return nil
	default:
		return v.Validate(cue.Concrete(true))
	}
}
