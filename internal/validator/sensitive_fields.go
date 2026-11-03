package validator

import (
	"bytes"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/fieldpath"
)

// sensitiveAllowlist is the struct form of a sensitive_fields entry: redact
// value in every element of the array each names, except the elements whose
// keep_when.field holds one of keep_when.in.
//
//	sensitive_fields: [{
//		each:  "metadata.items[*]"
//		value: "value"
//		keep_when: {field: "key", in: ["enable-oslogin"]}
//	}]
type sensitiveAllowlist struct {
	Each     string `json:"each"`
	Value    string `json:"value"`
	KeepWhen *struct {
		Field string    `json:"field"`
		In    *[]string `json:"in"`
	} `json:"keep_when"`
}

// decodeSensitiveFields reads _pudl.sensitive_fields: a list whose entries are
// field paths or allowlist structs. Each struct becomes the equivalent
// filtered path, so every consumer sees plain path strings. Any mistake is an
// error; callers fail closed.
func decodeSensitiveFields(v cue.Value) ([]string, error) {
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return nil, err
	}
	if v.IncompleteKind() != cue.ListKind {
		return nil, fmt.Errorf("must be a list of field paths or allowlist structs")
	}
	iter, err := v.List()
	if err != nil {
		return nil, err
	}
	var out []string
	for i := 0; iter.Next(); i++ {
		el := iter.Value()
		switch el.Kind() {
		case cue.StringKind:
			s, _ := el.String()
			out = append(out, s)
		case cue.StructKind:
			s, err := decodeAllowlist(el)
			if err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			out = append(out, s)
		default:
			return nil, fmt.Errorf("entry %d: must be a field path or an allowlist struct {each, value, keep_when}", i)
		}
	}
	return out, nil
}

func decodeAllowlist(el cue.Value) (string, error) {
	raw, err := el.MarshalJSON()
	if err != nil {
		return "", err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var a sensitiveAllowlist
	if err := dec.Decode(&a); err != nil {
		return "", fmt.Errorf("allowlist: %w", err)
	}
	switch {
	case a.Each == "":
		return "", fmt.Errorf("allowlist: each is required")
	case a.KeepWhen == nil:
		return "", fmt.Errorf("allowlist: keep_when is required (use a plain path to redact every element)")
	case a.KeepWhen.Field == "":
		return "", fmt.Errorf("allowlist: keep_when.field is required")
	case a.KeepWhen.In == nil:
		return "", fmt.Errorf("allowlist: keep_when.in is required")
	}
	p, err := fieldpath.ExceptWhere(a.Each, a.KeepWhen.Field, *a.KeepWhen.In, a.Value)
	if err != nil {
		return "", fmt.Errorf("allowlist: %w", err)
	}
	return p.String(), nil
}
