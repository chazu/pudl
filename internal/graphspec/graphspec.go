// Package graphspec parses and evaluates a schema's `_pudl.graph` block: how a
// record of the type is labelled in the resource graph (name, account,
// region, zone) and which values other records use to refer to it (refs).
//
// Each value is a field path, or {path, trim_prefix?, last_segment?, prepend?}
// (prepend is added after trimming, e.g. "serviceAccount:" to match how IAM
// bindings name a service account):
//
//	graph: {
//		name:    "name"
//		account: "project"
//		region:  {path: "region", last_segment: true}
//		refs: [{path: "selfLink", trim_prefix: "https://www.googleapis.com/compute/v1/"}]
//		display: {code: "VPC", label: "VPC network", class: "network"}
//		network: {group: true, via: ["in_project"]}
//	}
//
// display and the view roles (admin, network, port_of, security, inventory,
// application) tell pudl ui how to draw and place the type; see roles.go.
//
// Pure: no database or importer dependencies.
package graphspec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/chazu/pudl/internal/fieldpath"
)

// Value reads one string from a record.
type Value struct {
	Path        fieldpath.Path
	TrimPrefix  string
	LastSegment bool
	Prepend     string
}

// Spec is a decoded `_pudl.graph` block. Absent fields are nil.
type Spec struct {
	Name    *Value
	Account *Value
	Region  *Value
	Zone    *Value
	// Refs are the values other records use to name this resource, in
	// addition to its identity values.
	Refs []Value
	// Display is how pudl ui labels the type (nil when undeclared).
	Display *Display
	// Roles are the type's parts in pudl ui's views.
	Roles Roles
}

type rawSpec struct {
	Name    json.RawMessage   `json:"name"`
	Account json.RawMessage   `json:"account"`
	Region  json.RawMessage   `json:"region"`
	Zone    json.RawMessage   `json:"zone"`
	Refs    []json.RawMessage `json:"refs"`
	Display json.RawMessage   `json:"display"`
	rawRoles
}

type rawValue struct {
	Path        string `json:"path"`
	TrimPrefix  string `json:"trim_prefix"`
	LastSegment bool   `json:"last_segment"`
	Prepend     string `json:"prepend"`
}

// Parse decodes a `_pudl.graph` block strictly: unknown keys, wildcards and
// malformed paths are errors, so a typo cannot silently drop a label.
func Parse(raw json.RawMessage) (*Spec, error) {
	var r rawSpec
	if err := decodeStrict(raw, &r); err != nil {
		return nil, err
	}
	spec := &Spec{}
	for _, f := range []struct {
		name string
		raw  json.RawMessage
		dst  **Value
	}{{"name", r.Name, &spec.Name}, {"account", r.Account, &spec.Account}, {"region", r.Region, &spec.Region}, {"zone", r.Zone, &spec.Zone}} {
		if f.raw == nil {
			continue
		}
		v, err := parseValue(f.raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
		*f.dst = &v
	}
	for i, ref := range r.Refs {
		v, err := parseValue(ref)
		if err != nil {
			return nil, fmt.Errorf("refs[%d]: %w", i, err)
		}
		spec.Refs = append(spec.Refs, v)
	}
	if r.Display != nil {
		d, err := parseDisplay(r.Display)
		if err != nil {
			return nil, err
		}
		spec.Display = d
	}
	roles, err := parseRoles(r.rawRoles)
	if err != nil {
		return nil, err
	}
	spec.Roles = roles
	return spec, nil
}

func parseValue(raw json.RawMessage) (Value, error) {
	return ParseValue(raw, false)
}

// ParseValue decodes one value: a field path, or {path, trim_prefix?,
// last_segment?, prepend?}. wildcards permits a [*] in the path, for callers
// that fan out over it themselves (see Value.Apply).
func ParseValue(raw json.RawMessage, wildcards bool) (Value, error) {
	var rv rawValue
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		rv.Path = s
	} else if err := decodeStrict(raw, &rv); err != nil {
		return Value{}, fmt.Errorf("want a field path or {path, trim_prefix?, last_segment?, prepend?}: %w", err)
	}
	if rv.Path == "" {
		return Value{}, fmt.Errorf("path is required")
	}
	p, err := fieldpath.Parse(rv.Path)
	if err != nil {
		return Value{}, err
	}
	if p.HasWildcard() && !wildcards {
		return Value{}, fmt.Errorf("path %q: wildcards are not allowed", rv.Path)
	}
	return Value{Path: p, TrimPrefix: rv.TrimPrefix, LastSegment: rv.LastSegment, Prepend: rv.Prepend}, nil
}

func decodeStrict(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Eval reads the value from a decoded record. Only scalars yield a value.
func (v Value) Eval(record any) (string, bool) {
	raw, ok := v.Path.Lookup(record)
	if !ok {
		return "", false
	}
	return v.Apply(raw)
}

// Apply turns a value already read from a record (one element, for a path
// with a wildcard) into the string the spec yields. Only scalars yield one.
func (v Value) Apply(raw any) (string, bool) {
	s, ok := Scalar(raw)
	if !ok {
		return "", false
	}
	s = strings.TrimPrefix(s, v.TrimPrefix)
	if v.LastSegment {
		s = s[strings.LastIndex(s, "/")+1:]
	}
	if s == "" {
		return "", false
	}
	return v.Prepend + s, true
}

// Eval reads an optional value; a nil spec yields nothing.
func Eval(v *Value, record any) string {
	if v == nil {
		return ""
	}
	s, _ := v.Eval(record)
	return s
}

// RefValues returns the record's declared reference values.
func (s *Spec) RefValues(record any) []string {
	if s == nil {
		return nil
	}
	var out []string
	for _, ref := range s.Refs {
		if v, ok := ref.Eval(record); ok {
			out = append(out, v)
		}
	}
	return out
}

// Scalar renders a JSON scalar as a string.
func Scalar(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case json.Number:
		return x.String(), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	}
	return "", false
}
