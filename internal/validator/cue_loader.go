package validator

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

// CUEModuleLoader handles loading CUE modules with proper cross-reference support
type CUEModuleLoader struct {
	ctx        *cue.Context
	schemaPath string
	verbose    bool
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
		fmt.Printf("[CUE Loader] "+format+"\n", args...)
	}
}

// LoadedModule represents a loaded CUE module with its schemas and metadata
type LoadedModule struct {
	PackageName string                    `json:"package_name"`
	Schemas     map[string]cue.Value      `json:"-"` // Schema name -> CUE value
	Metadata    map[string]SchemaMetadata `json:"metadata"`
	LoadPath    string                    `json:"load_path"`
}

// LoadAllModules loads all CUE modules from the schema directory
// This method properly handles the new CUE module structure with hierarchical imports
// and third-party dependencies
func (loader *CUEModuleLoader) LoadAllModules() (map[string]*LoadedModule, error) {
	modules := make(map[string]*LoadedModule)

	loader.log("Loading CUE modules from: %s", loader.schemaPath)

	// Load the entire CUE module from the schema root
	// This handles all packages and their cross-references automatically,
	// including third-party dependencies from the module cache
	config := &load.Config{
		Dir: loader.schemaPath,
	}

	// Load all packages - we'll skip examples during processing
	instances := load.Instances([]string{"./..."}, config)

	if len(instances) == 0 {
		return nil, fmt.Errorf("no CUE instances found in schema module")
	}

	loader.log("Found %d CUE instances to load", len(instances))

	// Process each loaded instance (package)
	for _, inst := range instances {
		loader.log("Processing instance: %s (dir: %s)", inst.PkgName, inst.Dir)

		// Skip examples directory - it may have unfetched third-party dependencies
		if strings.Contains(inst.Dir, "/examples") || inst.PkgName == "examples" {
			loader.log("Skipping examples directory: %s", inst.Dir)
			continue
		}

		if inst.Err != nil {
			loader.log("Error loading instance %s: %v", inst.PkgName, inst.Err)
			return nil, fmt.Errorf("failed to load CUE instance %s: %w", inst.PkgName, inst.Err)
		}

		// Build the CUE value from the loaded instance
		value := loader.ctx.BuildInstance(inst)
		if value.Err() != nil {
			loader.log("Error building CUE value for %s: %v", inst.PkgName, value.Err())
			return nil, fmt.Errorf("failed to build CUE value for package %s: %w", inst.PkgName, value.Err())
		}

		// Create module from instance
		module, err := loader.createModuleFromInstance(inst, value)
		if err != nil {
			loader.log("Error creating module from %s: %v", inst.PkgName, err)
			return nil, fmt.Errorf("failed to create module from instance %s: %w", inst.PkgName, err)
		}

		loader.log("Successfully loaded module %s with %d schemas", inst.PkgName, len(module.Schemas))
		modules[inst.PkgName] = module
	}

	loader.log("Loaded %d modules total", len(modules))
	return modules, nil
}

// Legacy methods removed - no longer needed with new CUE module structure

// createModuleFromInstance creates a LoadedModule from a CUE instance
func (loader *CUEModuleLoader) createModuleFromInstance(inst *build.Instance, value cue.Value) (*LoadedModule, error) {
	schemas := make(map[string]cue.Value)
	metadata := make(map[string]SchemaMetadata)

	// Convert import path to module name (e.g., "pudl.schemas/aws/ec2" -> "aws/ec2")
	moduleName := strings.TrimPrefix(inst.ImportPath, "pudl.schemas/")
	if moduleName == inst.ImportPath {
		// Fallback to package name if not using module structure
		moduleName = inst.PkgName
	}

	// Iterate through all definitions in the package
	iter, err := value.Fields(cue.Definitions(true))
	if err != nil {
		return nil, fmt.Errorf("failed to iterate definitions in package %s: %w", inst.PkgName, err)
	}

	for iter.Next() {
		label := iter.Label()
		if !strings.HasPrefix(label, "#") {
			continue // Skip non-definition fields
		}

		schemaValue := iter.Value()
		// Use module-aware schema naming: "pudl.schemas/aws/ec2:#Instance"
		schemaName := fmt.Sprintf("pudl.schemas/%s:%s", moduleName, label)
		schemas[schemaName] = schemaValue

		// Extract PUDL metadata if present
		iter, err := schemaValue.Fields(cue.Hidden(true))
		if err == nil {
			for iter.Next() {
				if iter.Label() == "_pudl" {
					var meta SchemaMetadata
					if err := iter.Value().Decode(&meta); err == nil {
						metadata[schemaName] = meta
					}
					break
				}
			}
		}
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

// ValidateModuleIntegrity performs integrity checks on loaded modules
func (loader *CUEModuleLoader) ValidateModuleIntegrity(modules map[string]*LoadedModule) error {
	for packageName, module := range modules {
		// Check that the module has at least one schema
		if len(module.Schemas) == 0 {
			return fmt.Errorf("package %s contains no schema definitions", packageName)
		}

		// Validate that all schemas in the module are valid CUE values
		for schemaName, schemaValue := range module.Schemas {
			if err := schemaValue.Validate(); err != nil {
				return fmt.Errorf("schema %s failed validation: %w", schemaName, err)
			}
		}

		// Check for cross-reference consistency
		// If a schema has a base_schema in its metadata, verify it exists
		for schemaName, meta := range module.Metadata {
			if meta.BaseSchema != "" {
				// Check if the base schema exists in any loaded module
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
	}

	return nil
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
