package projection

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/chazu/pudl/internal/fieldpath"
	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
)

// SchemaSpec is a schema's effective projection: the relations declared along
// its base_schema chain (the nearer schema wins a name collision).
type SchemaSpec struct {
	Schema    string
	Relations []RelationSpec
	// Sensitive paths are redacted from a payload before projecting it, so a
	// backfill from data stored before a field became sensitive cannot copy it
	// into facts.
	Sensitive []fieldpath.Path
	// Fingerprint changes whenever anything that shapes the facts changes.
	Fingerprint string
	// Err disables projection for the schema; the import still succeeds and
	// reports it.
	Err error
}

// Registry resolves effective projection specs per schema.
type Registry struct {
	src      redact.Source
	redactor *redact.Registry
	mu       sync.Mutex
	cache    map[string]*SchemaSpec
}

// NewRegistry builds a registry over src (an *inference.SchemaInferrer).
// redactor may be nil, in which case one is built from src.
func NewRegistry(src redact.Source, redactor *redact.Registry) *Registry {
	if redactor == nil {
		redactor = redact.NewRegistry(src)
	}
	return &Registry{src: src, redactor: redactor, cache: map[string]*SchemaSpec{}}
}

// Schemas returns every loaded schema whose chain declares facts, sorted.
func (r *Registry) Schemas() []string {
	if r == nil || r.src == nil {
		return nil
	}
	var out []string
	for _, name := range r.src.GetAvailableSchemas() {
		if spec := r.For(name); spec != nil {
			out = append(out, schemaname.Normalize(name))
		}
	}
	sort.Strings(out)
	return out
}

// For returns schema's effective spec, or nil when its chain declares no facts.
func (r *Registry) For(schema string) *SchemaSpec {
	if r == nil || r.src == nil || schema == "" {
		return nil
	}
	schema = schemaname.Normalize(schema)
	r.mu.Lock()
	defer r.mu.Unlock()
	if spec, ok := r.cache[schema]; ok {
		return spec
	}
	spec := r.compute(schema)
	r.cache[schema] = spec
	return spec
}

func (r *Registry) compute(schema string) *SchemaSpec {
	own, ok := r.src.GetSchemaMetadata(schema)
	if !ok {
		return nil
	}
	spec := &SchemaSpec{Schema: schema}
	byName := map[string]RelationSpec{}
	declared := false
	for _, name := range r.src.GetInheritanceGraph().GetCascadeChain(schema) {
		meta, ok := r.src.GetSchemaMetadata(name)
		if !ok || (meta.FactsSpec == nil && meta.FactsError == "") {
			continue
		}
		declared = true
		if meta.FactsError != "" {
			spec.Err = fmt.Errorf("schema %s: %s", name, meta.FactsError)
			break
		}
		relations, err := ParseRelations(meta.FactsSpec)
		if err != nil {
			spec.Err = fmt.Errorf("schema %s: %w", name, err)
			break
		}
		for _, rel := range relations {
			if _, nearer := byName[rel.Name]; !nearer {
				byName[rel.Name] = rel
			}
		}
	}
	if !declared {
		return nil
	}
	if spec.Err == nil && len(own.IdentityFields) == 0 {
		// Without identity every changed record is a new resource, and facts
		// from every historical state would stay current.
		spec.Err = fmt.Errorf("schema %s declares facts but no identity_fields", schema)
	}
	if spec.Err == nil {
		spec.Sensitive, spec.Err = r.redactor.For(schema)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec.Relations = append(spec.Relations, byName[name])
	}
	spec.Fingerprint = fingerprint(spec)
	return spec
}

// fingerprint hashes everything that determines a schema's facts.
func fingerprint(spec *SchemaSpec) string {
	type argPrint struct {
		Path, Trim string
		Exists     bool
		Default    any
		HasDefault bool
	}
	type relPrint struct {
		Name, Each string
		Args       map[string]argPrint
	}
	print := struct {
		Relations []relPrint
		Sensitive []string
		Err       string
	}{}
	for _, rel := range spec.Relations {
		rp := relPrint{Name: rel.Name, Args: map[string]argPrint{}}
		if rel.Each != nil {
			rp.Each = rel.Each.String()
		}
		for name, a := range rel.Args {
			rp.Args[name] = argPrint{Path: a.Path.String(), Trim: a.TrimPrefix, Exists: a.Exists, Default: a.Default, HasDefault: a.HasDefault}
		}
		print.Relations = append(print.Relations, rp)
	}
	for _, p := range spec.Sensitive {
		print.Sensitive = append(print.Sensitive, p.String())
	}
	sort.Strings(print.Sensitive)
	if spec.Err != nil {
		print.Err = spec.Err.Error()
	}
	b, _ := json.Marshal(print)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// RelationArgs returns, for every relation any loaded schema projects, the
// args its facts carry (implicit ones included). The rule lint uses it.
func (r *Registry) RelationArgs() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, schema := range r.Schemas() {
		spec := r.For(schema)
		if spec == nil || spec.Err != nil {
			continue
		}
		for _, rel := range spec.Relations {
			args := out[rel.Name]
			if args == nil {
				args = map[string]bool{ArgEntryID: true, ArgResourceID: true}
				out[rel.Name] = args
			}
			for _, name := range rel.ArgNames {
				args[name] = true
			}
		}
	}
	return out
}
