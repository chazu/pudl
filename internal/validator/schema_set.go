package validator

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"cuelang.org/go/cue"
)

// SchemaSet is the merged view of one or more schema paths: every schema that
// loaded, with first-found-wins shadowing (per-repo shadows global), plus every
// package that did not load and every base_schema cycle.
//
// The inferrer and the chain validator each used to carry their own copy of
// this merge loop, and both skipped a failed path with a bare `continue`.
type SchemaSet struct {
	Loaders  []*CUEModuleLoader
	Modules  map[string]*LoadedModule
	Schemas  map[string]cue.Value
	Metadata map[string]SchemaMetadata
	Sources  map[string]string
	Shadowed map[string][]string
	// Errors lists the packages (or whole paths) that failed to load, and
	// schemas that loaded but failed integrity checks.
	Errors []SchemaLoadError
	// Cycles lists base_schema cycles in the merged metadata.
	Cycles [][]string
}

// LoadSchemaSet loads every schema path in order and merges the results.
//
// A path that does not exist, or holds no CUE packages, contributes nothing and
// is not an error: the global schema directory is optional. Every other failure
// is recorded in Errors rather than hiding the path's good packages.
func LoadSchemaSet(schemaPaths []string) *SchemaSet {
	set := &SchemaSet{
		Modules:  make(map[string]*LoadedModule),
		Schemas:  make(map[string]cue.Value),
		Metadata: make(map[string]SchemaMetadata),
		Sources:  make(map[string]string),
		Shadowed: make(map[string][]string),
	}
	moduleRoots := make(map[string]string)

	for _, sp := range schemaPaths {
		// Shared: two callers naming the same schema path share one compile, and
		// one caller loading twice compiles once. See module_cache.go.
		loader := SharedLoader(sp)
		set.Loaders = append(set.Loaders, loader)

		if _, err := os.Stat(sp); errors.Is(err, os.ErrNotExist) {
			continue
		}

		modules, loadErrs, err := loader.LoadModules()
		set.Errors = append(set.Errors, loadErrs...)
		if err != nil {
			if !errors.Is(err, ErrNoInstances) {
				set.Errors = append(set.Errors, SchemaLoadError{Path: sp, Err: err})
			}
			continue
		}
		set.Errors = append(set.Errors, loader.CheckModuleIntegrity(modules)...)

		for name, val := range loader.GetAllSchemas(modules) {
			if _, exists := set.Schemas[name]; !exists {
				set.Schemas[name] = val
				set.Sources[name] = sp
			} else if set.Sources[name] != sp {
				set.Shadowed[name] = append(set.Shadowed[name], sp)
			}
		}
		for name, meta := range loader.GetAllMetadata(modules) {
			if _, exists := set.Metadata[name]; !exists {
				set.Metadata[name] = meta
			}
		}
		for name, mod := range modules {
			if _, exists := set.Modules[name]; !exists {
				set.Modules[name] = mod
				moduleRoots[name] = sp
			}
		}
	}

	set.Errors = append(set.Errors, missingBaseSchemaErrors(set, moduleRoots)...)
	sort.SliceStable(set.Errors, func(i, j int) bool { return set.Errors[i].Error() < set.Errors[j].Error() })
	set.Cycles = FindBaseSchemaCycles(set.Metadata)
	return set
}

// missingBaseSchemaErrors reports schemas whose base_schema is not loaded.
//
// Checked across the merged set, not per path: a repository schema may extend a
// global one. The per-path check this replaces made the chain validator drop the
// whole repository path when it did.
func missingBaseSchemaErrors(set *SchemaSet, moduleRoots map[string]string) []SchemaLoadError {
	var problems []SchemaLoadError
	for name, mod := range set.Modules {
		for schemaName, meta := range mod.Metadata {
			if meta.BaseSchema == "" {
				continue
			}
			if _, exists := set.Schemas[meta.BaseSchema]; exists {
				continue
			}
			problems = append(problems, SchemaLoadError{
				Path:    moduleRoots[name],
				Package: mod.PackageName,
				Err:     fmt.Errorf("schema %s references non-existent base schema %s", schemaName, meta.BaseSchema),
			})
		}
	}
	return problems
}
