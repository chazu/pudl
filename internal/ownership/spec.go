// Package ownership derives who owns a resource from a schema's
// `_pudl.owner` declaration, and attributes findings to those owners.
//
// A declaration is tried in order: each field path (a label or tag), then a
// mapping table keyed by one field's value (a project to its team), then a
// fallback owner. A resource none of them names is unowned. The declaration
// is inherited along base_schema; the nearest schema that declares one wins.
//
// Ownership is projected at ingest as the built-in relations resource_owner
// and resource_unowned (see internal/projection), so rules and checks can
// join or list them. The resolution here is pure: no database access.
package ownership

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/chazu/pudl/internal/fieldpath"
)

// Spec is a decoded `_pudl.owner` block.
type Spec struct {
	// Paths are tried in order; the first that holds a non-empty string
	// names the owner.
	Paths []fieldpath.Path
	// Map, when set, names the owner from the value at Map.Path.
	Map *Mapping
	// Fallback owns what nothing else names; "" leaves it unowned.
	Fallback string
}

// Mapping is a lookup table from one field's value to an owner, e.g. a
// project ID to its team.
type Mapping struct {
	Path   fieldpath.Path
	Owners map[string]string
}

// Via values say how an owner was derived.
const (
	ViaPathPrefix = "path:"
	ViaMapPrefix  = "map:"
	ViaFallback   = "fallback"
)

// rawSpec is the JSON shape of `_pudl.owner`.
type rawSpec struct {
	Paths    []string `json:"paths"`
	Map      *rawMap  `json:"map"`
	Fallback *string  `json:"fallback"`
}

type rawMap struct {
	Path   string            `json:"path"`
	Owners map[string]string `json:"owners"`
}

// Parse strictly decodes a `_pudl.owner` block: {paths?: [path], map?:
// {path, owners: {value: owner}}, fallback?: owner}. Unknown keys, wildcard
// paths, empty owners and a block that names no way to derive an owner are
// errors.
func Parse(raw json.RawMessage) (*Spec, error) {
	var r rawSpec
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("must be {paths?: [path], map?: {path, owners}, fallback?: string}: %w", err)
	}
	spec := &Spec{}
	for _, p := range r.Paths {
		path, err := parsePath(p)
		if err != nil {
			return nil, fmt.Errorf("paths: %w", err)
		}
		spec.Paths = append(spec.Paths, path)
	}
	if r.Map != nil {
		path, err := parsePath(r.Map.Path)
		if err != nil {
			return nil, fmt.Errorf("map.path: %w", err)
		}
		if r.Map.Owners == nil {
			return nil, fmt.Errorf("map.owners is required (a struct of value: owner)")
		}
		for value, owner := range r.Map.Owners {
			if owner == "" {
				return nil, fmt.Errorf("map.owners.%q is empty", value)
			}
		}
		spec.Map = &Mapping{Path: path, Owners: r.Map.Owners}
	}
	if r.Fallback != nil {
		if *r.Fallback == "" {
			return nil, fmt.Errorf("fallback is empty; omit it to leave resources unowned")
		}
		spec.Fallback = *r.Fallback
	}
	if len(spec.Paths) == 0 && spec.Map == nil && spec.Fallback == "" {
		return nil, fmt.Errorf("declare at least one of paths, map or fallback")
	}
	return spec, nil
}

func parsePath(s string) (fieldpath.Path, error) {
	p, err := fieldpath.Parse(s)
	if err != nil {
		return fieldpath.Path{}, err
	}
	if p.HasWildcard() {
		return fieldpath.Path{}, fmt.Errorf("%q: an owner path names one value and cannot contain [*]", s)
	}
	return p, nil
}

// Resolve returns the owner of record and how it was derived. ok is false
// when the resource is unowned.
func (s *Spec) Resolve(record any) (owner, via string, ok bool) {
	if s == nil {
		return "", "", false
	}
	for _, p := range s.Paths {
		if v, found := p.Lookup(record); found {
			if str, isString := v.(string); isString && str != "" {
				return str, ViaPathPrefix + p.String(), true
			}
		}
	}
	if s.Map != nil {
		if v, found := s.Map.Path.Lookup(record); found {
			if key, isString := v.(string); isString {
				if owner := s.Map.Owners[key]; owner != "" {
					return owner, ViaMapPrefix + s.Map.Path.String(), true
				}
			}
		}
	}
	if s.Fallback != "" {
		return s.Fallback, ViaFallback, true
	}
	return "", "", false
}

// Canonical is a stable encoding of the spec, for fingerprints.
func (s *Spec) Canonical() string {
	if s == nil {
		return ""
	}
	out := struct {
		Paths    []string   `json:"paths,omitempty"`
		Map      [][]string `json:"map,omitempty"`
		MapPath  string     `json:"map_path,omitempty"`
		Fallback string     `json:"fallback,omitempty"`
	}{Fallback: s.Fallback}
	for _, p := range s.Paths {
		out.Paths = append(out.Paths, p.String())
	}
	if s.Map != nil {
		out.MapPath = s.Map.Path.String()
		out.Map = [][]string{}
		for value, owner := range s.Map.Owners {
			out.Map = append(out.Map, []string{value, owner})
		}
		sort.Slice(out.Map, func(i, j int) bool { return out.Map[i][0] < out.Map[j][0] })
	}
	b, _ := json.Marshal(out) // strings only
	return string(b)
}
