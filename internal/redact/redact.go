// Package redact removes values a schema declares sensitive
// (`_pudl.sensitive_fields`) before a record is hashed, stored, identified or
// projected. It fails closed: a schema whose sensitive declaration cannot be
// trusted makes every record routed to it an error rather than a stored secret.
package redact

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// Placeholder replaces every sensitive value.
const Placeholder = "[REDACTED]"

// Source is the schema metadata the registry reads; *inference.SchemaInferrer
// satisfies it.
type Source interface {
	GetAvailableSchemas() []string
	GetSchemaMetadata(string) (validator.SchemaMetadata, bool)
	GetInheritanceGraph() *inference.InheritanceGraph
}

// Registry resolves each schema's effective sensitive paths: the union along
// its base_schema chain.
type Registry struct {
	src   Source
	mu    sync.Mutex
	cache map[string]resolved
	any   *bool
}

type resolved struct {
	paths []fieldpath.Path
	err   error
}

// NewRegistry builds a registry over src. A nil src yields an empty registry.
func NewRegistry(src Source) *Registry {
	return &Registry{src: src, cache: map[string]resolved{}}
}

// Any reports whether any loaded schema declares sensitive fields (or a broken
// declaration). Inference may assign any schema, so this is the import gate.
func (r *Registry) Any() bool {
	if r == nil || r.src == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.any == nil {
		found := false
		for _, name := range r.src.GetAvailableSchemas() {
			if meta, ok := r.src.GetSchemaMetadata(name); ok && (len(meta.SensitiveFields) > 0 || meta.SensitiveError != "") {
				found = true
				break
			}
		}
		r.any = &found
	}
	return *r.any
}

// For returns the union of the effective sensitive paths of every named
// schema. Empty names are ignored. An error means at least one schema's
// declaration is broken and records routed to it must not be stored.
func (r *Registry) For(schemas ...string) ([]fieldpath.Path, error) {
	if r == nil || r.src == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []fieldpath.Path
	for _, schema := range schemas {
		if schema == "" {
			continue
		}
		res := r.resolve(schemaname.Normalize(schema))
		if res.err != nil {
			return nil, res.err
		}
		for _, p := range res.paths {
			if !seen[p.String()] {
				seen[p.String()] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func (r *Registry) resolve(schema string) resolved {
	r.mu.Lock()
	defer r.mu.Unlock()
	if res, ok := r.cache[schema]; ok {
		return res
	}
	res := r.compute(schema)
	r.cache[schema] = res
	return res
}

func (r *Registry) compute(schema string) resolved {
	own, ok := r.src.GetSchemaMetadata(schema)
	if !ok {
		return resolved{}
	}
	var paths []fieldpath.Path
	seen := map[string]bool{}
	for _, name := range r.src.GetInheritanceGraph().GetCascadeChain(schema) {
		meta, ok := r.src.GetSchemaMetadata(name)
		if !ok {
			continue
		}
		if meta.SensitiveError != "" {
			return resolved{err: fmt.Errorf("schema %s: %s", name, meta.SensitiveError)}
		}
		for _, raw := range meta.SensitiveFields {
			p, err := fieldpath.Parse(raw)
			if err != nil {
				return resolved{err: fmt.Errorf("schema %s: sensitive_fields: %w", name, err)}
			}
			if !seen[raw] {
				seen[raw] = true
				paths = append(paths, p)
			}
		}
	}
	// Redacting part of an identity would give every record the same identity
	// and collapse them into one resource's version chain.
	for _, id := range own.IdentityFields {
		idPath, err := fieldpath.Parse(id)
		if err != nil {
			continue // identity reporting covers unparsable identity fields
		}
		for _, p := range paths {
			if p.Overlaps(idPath) {
				return resolved{err: fmt.Errorf("schema %s: sensitive field %q overlaps identity field %q", schema, p.String(), id)}
			}
		}
	}
	return resolved{paths: paths}
}

// Apply replaces every value at paths in record with Placeholder and returns
// how many were replaced.
func Apply(record any, paths []fieldpath.Path) int {
	n := 0
	for _, p := range paths {
		n += p.Replace(record, func(any) any { return Placeholder })
	}
	return n
}

// Describe lists paths for messages, sorted.
func Describe(paths []fieldpath.Path) string {
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = p.String()
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
