package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

// CUEModuleLoader handles loading CUE modules with proper cross-reference support
type CUEModuleLoader struct {
	ctx        *cue.Context
	schemaPath string
}

// NewCUEModuleLoader creates a new CUE module loader
func NewCUEModuleLoader(schemaPath string) *CUEModuleLoader {
	return &CUEModuleLoader{
		ctx:        cuecontext.New(),
		schemaPath: schemaPath,
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
// This method properly handles cross-references within packages using CUE's load package
func (loader *CUEModuleLoader) LoadAllModules() (map[string]*LoadedModule, error) {
	modules := make(map[string]*LoadedModule)

	// Discover all package directories
	packageDirs, err := loader.discoverPackageDirectories()
	if err != nil {
		return nil, fmt.Errorf("failed to discover package directories: %w", err)
	}

	// Load each package as a CUE module
	for packageName, packageDir := range packageDirs {
		module, err := loader.loadPackageModule(packageName, packageDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load package %s: %w", packageName, err)
		}
		modules[packageName] = module
	}

	return modules, nil
}

// discoverPackageDirectories finds all package directories in the schema path
func (loader *CUEModuleLoader) discoverPackageDirectories() (map[string]string, error) {
	packageDirs := make(map[string]string)

	entries, err := os.ReadDir(loader.schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			packagePath := filepath.Join(loader.schemaPath, entry.Name())
			
			// Check if directory contains CUE files
			if hasCUEFiles, err := loader.hasCUEFiles(packagePath); err != nil {
				return nil, fmt.Errorf("failed to check CUE files in %s: %w", packagePath, err)
			} else if hasCUEFiles {
				packageDirs[entry.Name()] = packagePath
			}
		}
	}

	return packageDirs, nil
}

// hasCUEFiles checks if a directory contains any .cue files
func (loader *CUEModuleLoader) hasCUEFiles(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".cue") {
			return true, nil
		}
	}

	return false, nil
}

// loadPackageModule loads a single package as a CUE module with cross-reference support
func (loader *CUEModuleLoader) loadPackageModule(packageName, packageDir string) (*LoadedModule, error) {
	// Use CUE's load package to properly handle module loading
	// This automatically resolves cross-references within the package
	config := &load.Config{
		Dir: packageDir,
	}

	// Load all CUE files in the package directory
	instances := load.Instances([]string{"."}, config)
	if len(instances) == 0 {
		return nil, fmt.Errorf("no CUE instances found in package %s", packageName)
	}

	// Check for load errors
	for _, inst := range instances {
		if inst.Err != nil {
			return nil, fmt.Errorf("failed to load CUE instance in package %s: %w", packageName, inst.Err)
		}
	}

	// Build the CUE value from the loaded instance
	// For packages with multiple files, CUE automatically handles unification
	inst := instances[0] // Packages should have one unified instance
	value := loader.ctx.BuildInstance(inst)
	if value.Err() != nil {
		return nil, fmt.Errorf("failed to build CUE value for package %s: %w", packageName, value.Err())
	}

	// Extract schemas and metadata from the unified package value
	schemas := make(map[string]cue.Value)
	metadata := make(map[string]SchemaMetadata)

	// Iterate through all definitions in the package
	iter, err := value.Fields(cue.Definitions(true))
	if err != nil {
		return nil, fmt.Errorf("failed to iterate definitions in package %s: %w", packageName, err)
	}

	for iter.Next() {
		label := iter.Label()
		if !strings.HasPrefix(label, "#") {
			continue // Skip non-definition fields
		}

		schemaValue := iter.Value()
		schemaName := fmt.Sprintf("%s.%s", packageName, label)
		schemas[schemaName] = schemaValue

		// Extract PUDL metadata if present
		// Access hidden fields by iterating through all fields including hidden ones
		// This is necessary because CUE considers fields starting with _ as hidden
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
		PackageName: packageName,
		Schemas:     schemas,
		Metadata:    metadata,
		LoadPath:    packageDir,
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
