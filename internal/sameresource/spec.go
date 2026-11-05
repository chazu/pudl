// Package sameresource links records of different schemas that describe one
// real resource, without merging them. See
// docs/design/2026-11-05-shared-identity-across-schemas.md.
//
// A schema declares the keys its records hold in `_pudl.same_resource`:
//
//	same_resource: [
//		{kind: "k8s.cluster", key: {host: "endpoint"}},
//		{kind: "k8s.cluster", key: {host: "privateClusterConfig.privateEndpoint"}},
//	]
//
// Each key value is a field path or {path, trim_prefix?, last_segment?,
// prepend?}, as in `_pudl.graph` (see graphspec). At most one value of a
// declaration may contain [*]; it yields one key per element. Projection
// writes one same_resource_key(kind, key, resource_id) fact per key, and the
// built-in rule same_resource_by_key links resources with equal keys.
//
// Pure: no database or importer dependencies.
package sameresource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/graphspec"
	"github.com/chazu/pudl/internal/identity"
)

// Relation names and the args of same_resource_key beside the implicit
// entry_id and resource_id. The database package reserves the relation
// names; a test keeps them aligned.
const (
	LinkRelation = "same_resource"
	KeyRelation  = "same_resource_key"
	KindRelation = "same_resource_kind"

	ArgKind = "kind"
	ArgKey  = "key"
)

var (
	kindPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)
	keyName     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Decl is one declared key: a kind and how to read each key part.
type Decl struct {
	Kind     string
	KeyNames []string // sorted
	Key      map[string]graphspec.Value
	// wildcard names the one part whose path fans out, if any.
	wildcard string
}

// Spec is a decoded `_pudl.same_resource` block.
type Spec struct {
	Decls []Decl
}

// Key is one key a record holds: its kind and the canonical JSON of its parts.
type Key struct {
	Kind string
	Key  string
}

type rawDecl struct {
	Kind string                     `json:"kind"`
	Key  map[string]json.RawMessage `json:"key"`
}

// Parse decodes a `_pudl.same_resource` block strictly: unknown fields, a
// malformed kind, an empty key or two wildcard parts are errors, so a typo
// cannot silently drop a link.
func Parse(raw json.RawMessage) (*Spec, error) {
	var decls []json.RawMessage
	if err := json.Unmarshal(raw, &decls); err != nil {
		return nil, fmt.Errorf("want a list of {kind, key}: %w", err)
	}
	if len(decls) == 0 {
		return nil, fmt.Errorf("want at least one {kind, key}")
	}
	spec := &Spec{}
	for i, d := range decls {
		decl, err := parseDecl(d)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		spec.Decls = append(spec.Decls, decl)
	}
	return spec, nil
}

func parseDecl(raw json.RawMessage) (Decl, error) {
	var r rawDecl
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Decl{}, fmt.Errorf("want {kind, key}: %w", err)
	}
	if !kindPattern.MatchString(r.Kind) {
		return Decl{}, fmt.Errorf("kind %q must be dotted lower-case words, e.g. k8s.cluster", r.Kind)
	}
	if len(r.Key) == 0 {
		return Decl{}, fmt.Errorf("kind %s: key needs at least one part", r.Kind)
	}
	decl := Decl{Kind: r.Kind, Key: map[string]graphspec.Value{}}
	for name, rawValue := range r.Key {
		if !keyName.MatchString(name) {
			return Decl{}, fmt.Errorf("kind %s: key part %q must be an identifier", r.Kind, name)
		}
		v, err := graphspec.ParseValue(rawValue, true)
		if err != nil {
			return Decl{}, fmt.Errorf("kind %s: key part %s: %w", r.Kind, name, err)
		}
		if v.Path.HasWildcard() {
			if decl.wildcard != "" {
				return Decl{}, fmt.Errorf("kind %s: only one key part may contain [*] (%s and %s do)", r.Kind, decl.wildcard, name)
			}
			decl.wildcard = name
		}
		decl.Key[name] = v
		decl.KeyNames = append(decl.KeyNames, name)
	}
	sort.Strings(decl.KeyNames)
	return decl, nil
}

// Keys returns the distinct keys record holds, sorted. A declaration whose
// parts are not all present yields no key: absent data never links.
func (s *Spec) Keys(record any) []Key {
	if s == nil {
		return nil
	}
	seen := map[Key]bool{}
	var out []Key
	for _, d := range s.Decls {
		for _, parts := range d.parts(record) {
			canonical, err := identity.CanonicalIdentityJSON(parts)
			if err != nil {
				continue
			}
			k := Key{Kind: d.Kind, Key: canonical}
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// parts returns the key part maps one declaration yields from record.
func (d Decl) parts(record any) []map[string]any {
	base := map[string]any{}
	for _, name := range d.KeyNames {
		if name == d.wildcard {
			continue
		}
		v, ok := d.Key[name].Eval(record)
		if !ok {
			return nil
		}
		base[name] = v
	}
	if d.wildcard == "" {
		return []map[string]any{base}
	}
	spec := d.Key[d.wildcard]
	var out []map[string]any
	for _, raw := range spec.Path.LookupAll(record) {
		v, ok := spec.Apply(raw)
		if !ok {
			continue
		}
		parts := make(map[string]any, len(base)+1)
		for k, bv := range base {
			parts[k] = bv
		}
		parts[d.wildcard] = v
		out = append(out, parts)
	}
	return out
}

// Canonical renders the spec deterministically, for projection fingerprints.
func (s *Spec) Canonical() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	for _, d := range s.Decls {
		b.WriteString(d.Kind)
		for _, name := range d.KeyNames {
			v := d.Key[name]
			fmt.Fprintf(&b, "|%s=%s;%s;%t;%s", name, v.Path.String(), v.TrimPrefix, v.LastSegment, v.Prepend)
		}
		b.WriteString("\n")
	}
	return b.String()
}
