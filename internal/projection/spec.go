// Package projection turns imported records into facts, as declared by a
// schema's `_pudl.facts` block, so Datalog rules and #Checks can see imported
// fields. See docs/design/2026-10-08-gcp-cataloging-ux.md §5.
package projection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/fieldpath"
)

// Implicit args every projected fact carries.
const (
	ArgEntryID    = "entry_id"
	ArgResourceID = "resource_id"
)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ArgSpec is how one fact arg is read from a row.
type ArgSpec struct {
	Path       fieldpath.Path
	Exists     bool // the arg is whether Path yields anything
	Default    any
	HasDefault bool
	TrimPrefix string
}

// RelationSpec is one projected relation.
type RelationSpec struct {
	Name     string
	Each     *fieldpath.Path // rows; nil means the record is the only row
	ArgNames []string        // sorted
	Args     map[string]ArgSpec
	// wildcard names the one arg whose path fans out, if any.
	wildcard string
}

// rawRelation is the JSON shape of one relation in `_pudl.facts`.
type rawRelation struct {
	Each string                     `json:"each"`
	Args map[string]json.RawMessage `json:"args"`
}

type rawArg struct {
	Path       *string         `json:"path"`
	Exists     *string         `json:"exists"`
	Default    json.RawMessage `json:"default"`
	TrimPrefix string          `json:"trim_prefix"`
}

// ParseRelations decodes and validates a `_pudl.facts` block.
func ParseRelations(raw json.RawMessage) ([]RelationSpec, error) {
	var relations map[string]json.RawMessage
	if err := decodeStrict(raw, &relations); err != nil {
		return nil, fmt.Errorf("facts must map relation names to {each?, args}: %w", err)
	}
	names := make([]string, 0, len(relations))
	for name := range relations {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]RelationSpec, 0, len(names))
	for _, name := range names {
		rel, err := parseRelation(name, relations[name])
		if err != nil {
			return nil, fmt.Errorf("facts.%s: %w", name, err)
		}
		out = append(out, rel)
	}
	return out, nil
}

func parseRelation(name string, raw json.RawMessage) (RelationSpec, error) {
	if !identifier.MatchString(name) {
		return RelationSpec{}, fmt.Errorf("relation name must be an identifier ([A-Za-z_][A-Za-z0-9_]*)")
	}
	if database.IsReservedRelation(name) {
		return RelationSpec{}, fmt.Errorf("relation name %q is reserved", name)
	}
	var r rawRelation
	if err := decodeStrict(raw, &r); err != nil {
		return RelationSpec{}, fmt.Errorf("expected {each?: path, args: {...}}: %w", err)
	}
	if len(r.Args) == 0 {
		return RelationSpec{}, fmt.Errorf("args must declare at least one arg")
	}
	rel := RelationSpec{Name: name, Args: map[string]ArgSpec{}}
	if r.Each != "" {
		each, err := fieldpath.Parse(r.Each)
		if err != nil {
			return RelationSpec{}, fmt.Errorf("each: %w", err)
		}
		if !each.HasWildcard() {
			return RelationSpec{}, fmt.Errorf("each %q must contain a [*] wildcard", r.Each)
		}
		rel.Each = &each
	}
	for argName, rawSpec := range r.Args {
		if !identifier.MatchString(argName) {
			return RelationSpec{}, fmt.Errorf("arg name %q must be an identifier", argName)
		}
		if argName == ArgEntryID || argName == ArgResourceID {
			return RelationSpec{}, fmt.Errorf("arg name %q is implicit and cannot be declared", argName)
		}
		spec, err := parseArg(rawSpec)
		if err != nil {
			return RelationSpec{}, fmt.Errorf("args.%s: %w", argName, err)
		}
		if !spec.Exists && spec.Path.HasWildcard() {
			if rel.wildcard != "" {
				return RelationSpec{}, fmt.Errorf("args %s and %s both contain [*]; use each: to pair values row by row", rel.wildcard, argName)
			}
			rel.wildcard = argName
		}
		rel.Args[argName] = spec
		rel.ArgNames = append(rel.ArgNames, argName)
	}
	sort.Strings(rel.ArgNames)
	return rel, nil
}

func parseArg(raw json.RawMessage) (ArgSpec, error) {
	var path string
	if err := json.Unmarshal(raw, &path); err == nil {
		p, err := fieldpath.Parse(path)
		if err != nil {
			return ArgSpec{}, err
		}
		return ArgSpec{Path: p}, nil
	}
	var r rawArg
	if err := decodeStrict(raw, &r); err != nil {
		return ArgSpec{}, fmt.Errorf("expected a path string, {path, default?, trim_prefix?} or {exists: path}: %w", err)
	}
	switch {
	case r.Exists != nil && r.Path == nil:
		if len(r.Default) > 0 || r.TrimPrefix != "" {
			return ArgSpec{}, fmt.Errorf("exists takes no default or trim_prefix")
		}
		p, err := fieldpath.Parse(*r.Exists)
		if err != nil {
			return ArgSpec{}, err
		}
		return ArgSpec{Path: p, Exists: true}, nil
	case r.Path != nil && r.Exists == nil:
		p, err := fieldpath.Parse(*r.Path)
		if err != nil {
			return ArgSpec{}, err
		}
		spec := ArgSpec{Path: p, TrimPrefix: r.TrimPrefix}
		if len(r.Default) > 0 {
			def, err := decodeExact(r.Default)
			if err != nil {
				return ArgSpec{}, fmt.Errorf("default: %w", err)
			}
			spec.Default, spec.HasDefault = def, true
		}
		return spec, nil
	default:
		return ArgSpec{}, fmt.Errorf("exactly one of path or exists is required")
	}
}

func decodeStrict(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	return dec.Decode(v)
}

func decodeExact(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}
