package validator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	"github.com/chazu/pudl/internal/schemaname"
)

// CUEModuleLoader handles loading CUE modules with proper cross-reference support
type CUEModuleLoader struct {
	ctx        *cue.Context
	schemaPath string
	verbose    bool

	// cacheMu guards cache. Nothing in pudl loads schemas concurrently today, but
	// a memo that is not race-safe is a trap for the first caller who does.
	cacheMu sync.Mutex
	cache   *cachedModules
}

// NewCUEModuleLoader creates a new CUE module loader
func NewCUEModuleLoader(schemaPath string) *CUEModuleLoader {
	return &CUEModuleLoader{
		ctx:        cuecontext.New(),
		schemaPath: schemaPath,
		verbose:    false,
	}
}

// SetVerbose enables or disables verbose logging
func (loader *CUEModuleLoader) SetVerbose(verbose bool) {
	loader.verbose = verbose
}

// log prints a message if verbose mode is enabled
func (loader *CUEModuleLoader) log(format string, args ...interface{}) {
	if loader.verbose {
		fmt.Fprintf(os.Stderr, "[CUE Loader] "+format+"\n", args...)
	}
}

// LoadedModule represents a loaded CUE module with its schemas and metadata
type LoadedModule struct {
	PackageName string                    `json:"package_name"`
	Schemas     map[string]cue.Value      `json:"-"` // Schema name -> CUE value
	Metadata    map[string]SchemaMetadata `json:"metadata"`
	LoadPath    string                    `json:"load_path"`
}

// LoadAllModules loads all CUE modules from the schema directory, failing if
// any package does not load. If any instances have missing dependencies, it
// runs "cue mod tidy" to fetch them and retries the load once.
//
// Callers that must see a broken package as an error (resolving a named model)
// use this. Callers that should keep working with the packages that did load
// (inference, validation) use LoadModules.
func (loader *CUEModuleLoader) LoadAllModules() (map[string]*LoadedModule, error) {
	modules, loadErrs, err := loader.LoadModules()
	if err != nil {
		return nil, err
	}
	if len(loadErrs) > 0 {
		return nil, loadErrs[0].Err
	}
	return modules, nil
}

// LoadModules loads every package in the schema directory that can be loaded,
// and reports each one that cannot. The error return is reserved for a failure
// of the path as a whole (ErrNoInstances, an unreadable directory).
func (loader *CUEModuleLoader) LoadModules() (map[string]*LoadedModule, []SchemaLoadError, error) {
	return loader.loadAllModulesCached()
}

// LoadErrors reports the packages under this loader's schema path that do not
// load, or the path's own failure. Served from the same memo as LoadModules.
func (loader *CUEModuleLoader) LoadErrors() []SchemaLoadError {
	_, loadErrs, err := loader.LoadModules()
	if err != nil {
		return []SchemaLoadError{{Path: loader.schemaPath, Err: err}}
	}
	return loadErrs
}

// loadAllModulesUncached is the real load, behind the memo in module_cache.go.
func (loader *CUEModuleLoader) loadAllModulesUncached() (map[string]*LoadedModule, []SchemaLoadError, error) {
	moduleLoads.Add(1)
	modules, loadErrs, missing, err := loader.loadAllModulesOnce()
	if err != nil {
		return nil, nil, err
	}
	if len(missing) == 0 {
		return modules, loadErrs, nil
	}

	loader.log("Missing CUE dependencies detected, running cue mod tidy")
	if tidyErr := loader.runCueModTidy(); tidyErr != nil {
		for _, m := range missing {
			m.Err = fmt.Errorf("missing CUE dependencies and cue mod tidy failed: %w (load error: %v)", tidyErr, m.Err)
			loadErrs = append(loadErrs, m)
		}
		return modules, loadErrs, nil
	}
	modules, loadErrs, missing, err = loader.loadAllModulesOnce()
	if err != nil {
		return nil, nil, err
	}
	return modules, append(loadErrs, missing...), nil
}

// loadAllModulesOnce attempts a single load pass, one package at a time. A
// package that fails is reported and skipped; the rest still load. Packages
// that fail only for missing dependencies are returned separately in missing,
// because "cue mod tidy" may resolve them.
func (loader *CUEModuleLoader) loadAllModulesOnce() (modules map[string]*LoadedModule, loadErrs, missing []SchemaLoadError, err error) {
	modules = make(map[string]*LoadedModule)

	loader.log("Loading CUE modules from: %s", loader.schemaPath)

	config := &load.Config{
		Dir: loader.schemaPath,
	}

	instances := load.Instances([]string{"./..."}, config)

	if len(instances) == 0 {
		return nil, nil, nil, ErrNoInstances
	}

	loader.log("Found %d CUE instances to load", len(instances))

	for _, inst := range instances {
		name := loader.moduleName(inst)
		loader.log("Processing instance: %s (dir: %s)", name, inst.Dir)
		fail := func(err error) {
			loader.log("Error loading %s: %v", name, err)
			loadErrs = append(loadErrs, SchemaLoadError{Path: loader.schemaPath, Package: name, Err: err})
		}

		if inst.Err != nil {
			if isMissingDependencyErr(inst.Err) {
				missing = append(missing, SchemaLoadError{Path: loader.schemaPath, Package: name, Err: inst.Err})
				continue
			}
			fail(fmt.Errorf("failed to load CUE instance %s: %w", inst.PkgName, inst.Err))
			continue
		}

		// Build the CUE value from the loaded instance
		value := loader.ctx.BuildInstance(inst)
		if value.Err() != nil {
			fail(fmt.Errorf("failed to build CUE value for package %s: %w", inst.PkgName, value.Err()))
			continue
		}

		module, err := loader.createModuleFromInstance(inst, value)
		if err != nil {
			fail(fmt.Errorf("failed to create module from instance %s: %w", inst.PkgName, err))
			continue
		}

		// Keyed by the schema-root-relative directory, not the CUE package name:
		// pudl/k8s and vendor/k8s are both package "k8s", and keying by package
		// name let the second silently replace the first.
		loader.log("Successfully loaded module %s with %d schemas", name, len(module.Schemas))
		modules[name] = module
	}

	loader.log("Loaded %d modules total", len(modules))
	return modules, loadErrs, missing, nil
}

// moduleName is the namespace a package's schemas are registered under: its
// schema-root-relative directory, falling back to the CUE package name for a
// package at the root itself.
//
// The schema directory is the authoritative namespace. CUE's package name
// collapses nested paths (pudl/k8s becomes just k8s), and module import paths
// vary between pudl.schemas/pudl/k8s and pudl.schemas@v0/pudl/k8s. Deriving the
// name from the directory keeps validation aligned with inference and catalog
// schema references.
func (loader *CUEModuleLoader) moduleName(inst *build.Instance) string {
	if rel, err := filepath.Rel(loader.schemaPath, inst.Dir); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return inst.PkgName
}

// isMissingDependencyErr returns true if the error indicates unfetched CUE
// packages or modules that "cue mod tidy" can resolve.
func isMissingDependencyErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "cannot find package") || strings.Contains(s, "cannot find module")
}

// runCueModTidy executes "cue mod tidy" in the schema directory to fetch
// missing third-party dependencies from the CUE module registry.
func (loader *CUEModuleLoader) runCueModTidy() error {
	cmd := exec.Command("cue", "mod", "tidy")
	cmd.Dir = loader.schemaPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	loader.log("cue mod tidy completed successfully")
	return nil
}

// Legacy methods removed - no longer needed with new CUE module structure

// createModuleFromInstance creates a LoadedModule from a CUE instance
func (loader *CUEModuleLoader) createModuleFromInstance(inst *build.Instance, value cue.Value) (*LoadedModule, error) {
	schemas := make(map[string]cue.Value)
	metadata := make(map[string]SchemaMetadata)

	moduleName := loader.moduleName(inst)

	// Iterate through all definitions in the package
	iter, err := value.Fields(cue.Definitions(true))
	if err != nil {
		return nil, fmt.Errorf("failed to iterate definitions in package %s: %w", inst.PkgName, err)
	}

	for iter.Next() {
		label := iter.Selector().String()
		if !strings.HasPrefix(label, "#") {
			continue // Skip non-definition fields
		}

		schemaValue := iter.Value()

		// Detect if schema is structurally a list type using CUE's IncompleteKind.
		// This allows us to identify collection schemas like `#CatchAllCollection: [...]`
		// without relying on metadata (which arrays can't have).
		isListType := (schemaValue.IncompleteKind() & cue.ListKind) != 0

		// Extract PUDL metadata if present
		var meta SchemaMetadata
		hasPudl := false
		innerIter, err := schemaValue.Fields(cue.Hidden(true))
		if err == nil {
			for innerIter.Next() {
				if innerIter.Selector().String() == "_pudl" {
					hasPudl = true
					// Best effort: a partially decodable block still yields the
					// fields that did decode, and the zero value otherwise.
					_ = innerIter.Value().Decode(&meta)
					decodeStrictPudlParts(innerIter.Value(), &meta)
					break
				}
			}
		}

		// A definition carrying no `_pudl` block is a *component* -- a reusable
		// shape meant to be embedded in a schema (e.g. `#Tag`, `#ServiceBinding`),
		// inert to inference -- not a tracked schema. Skip registering it so
		// components do not appear as phantom schemas in listings or inference
		// candidates. List-type schemas (collections) legitimately carry no
		// `_pudl` because arrays have no fields, so they are exempt.
		if !hasPudl && !isListType {
			continue
		}

		// Use canonical schema naming: "aws/ec2.#Instance"
		// The schemaname.Format function strips version suffixes like @v0
		canonicalName := schemaname.Format(moduleName, label)
		schemas[canonicalName] = schemaValue

		// Set the IsListType field based on structural detection
		meta.IsListType = isListType
		metadata[canonicalName] = meta
	}

	return &LoadedModule{
		PackageName: moduleName,
		Schemas:     schemas,
		Metadata:    metadata,
		LoadPath:    inst.Dir,
	}, nil
}

// GetAllSchemas returns a flattened map of all schemas from all loaded modules
func (loader *CUEModuleLoader) GetAllSchemas(modules map[string]*LoadedModule) map[string]cue.Value {
	allSchemas := make(map[string]cue.Value)

	for _, module := range modules {
		for schemaName, schemaValue := range module.Schemas {
			allSchemas[schemaName] = schemaValue
		}
	}

	return allSchemas
}

// GetAllMetadata returns a flattened map of all metadata from all loaded modules
func (loader *CUEModuleLoader) GetAllMetadata(modules map[string]*LoadedModule) map[string]SchemaMetadata {
	allMetadata := make(map[string]SchemaMetadata)

	for _, module := range modules {
		for schemaName, metadata := range module.Metadata {
			allMetadata[schemaName] = metadata
		}
	}

	return allMetadata
}

// ValidateModuleIntegrity performs integrity checks on loaded modules,
// returning the first problem found. LoadSchemaSet uses CheckModuleIntegrity
// instead, which reports every problem without discarding the path.
func (loader *CUEModuleLoader) ValidateModuleIntegrity(modules map[string]*LoadedModule) error {
	if problems := loader.CheckModuleIntegrity(modules); len(problems) > 0 {
		return problems[0].Err
	}
	for _, module := range modules {
		// If a schema has a base_schema in its metadata, verify it exists in
		// this path's modules.
		for schemaName, meta := range module.Metadata {
			if meta.BaseSchema == "" {
				continue
			}
			baseSchemaExists := false
			for _, otherModule := range modules {
				if _, exists := otherModule.Schemas[meta.BaseSchema]; exists {
					baseSchemaExists = true
					break
				}
			}
			if !baseSchemaExists {
				return fmt.Errorf("schema %s references non-existent base schema %s", schemaName, meta.BaseSchema)
			}
		}
	}
	return nil
}

// CheckModuleIntegrity reports every registered schema whose CUE value does
// not validate. A package may contain only reusable components or rule
// definitions; those are valid parts of the schema tree even though they are
// not independently registered validation targets.
//
// base_schema references are not checked here: a schema in one path may extend
// one in another, so LoadSchemaSet checks them across the merged set.
func (loader *CUEModuleLoader) CheckModuleIntegrity(modules map[string]*LoadedModule) []SchemaLoadError {
	var problems []SchemaLoadError
	for name, module := range modules {
		for schemaName, schemaValue := range module.Schemas {
			if err := schemaValue.Validate(); err != nil {
				problems = append(problems, SchemaLoadError{
					Path:    loader.schemaPath,
					Package: name,
					Err:     fmt.Errorf("schema %s failed validation: %w", schemaName, err),
				})
			}
		}
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].Error() < problems[j].Error() })
	return problems
}

// GetModuleInfo returns information about a specific loaded module
func (loader *CUEModuleLoader) GetModuleInfo(modules map[string]*LoadedModule, packageName string) (*LoadedModule, error) {
	module, exists := modules[packageName]
	if !exists {
		return nil, fmt.Errorf("module %s not found", packageName)
	}
	return module, nil
}

// GetSchemaFromModules retrieves a specific schema from the loaded modules
func (loader *CUEModuleLoader) GetSchemaFromModules(modules map[string]*LoadedModule, schemaName string) (cue.Value, error) {
	for _, module := range modules {
		if schema, exists := module.Schemas[schemaName]; exists {
			return schema, nil
		}
	}
	return cue.Value{}, fmt.Errorf("schema %s not found in any loaded module", schemaName)
}
