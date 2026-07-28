package mubridge

// Plugin package metadata is intentionally read from files rather than by
// importing mu's Go packages. mu and pudl remain independently installable;
// this small file/CUE contract is the bridge between them.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/muschemas"
)

// PluginSchemaDecl identifies a plugin-owned CUE schema module declared in
// the plugin's mu.cue manifest.
type PluginSchemaDecl struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Path    string `json:"path"`
}

// PluginMapping maps a plugin-emitted resource discriminator to a PUDL-owned
// semantic schema. The mapping is classification metadata; the plugin-owned
// wire schema and the PUDL schema need not have identical field shapes.
type PluginMapping struct {
	ResourceType string `json:"resource_type"`
	Schema       string `json:"schema"`
}

// PluginPackage is the PUDL-readable part of an installed mu plugin package.
type PluginPackage struct {
	Schemas  []PluginSchemaDecl
	Mappings []PluginMapping
}

type pluginManifestFile struct {
	Plugin struct {
		Schemas []PluginSchemaDecl `json:"schemas"`
	} `json:"plugin"`
}

type pluginMappingsFile struct {
	Mappings []PluginMapping `json:"mappings"`
}

// LoadPluginPackage reads mu.cue and the optional pudl.cue metadata from a
// plugin directory. A plugin without PUDL mappings is valid; it simply uses
// legacy resource routing or generic observe classification.
func LoadPluginPackage(pluginDir string) (PluginPackage, error) {
	manifest, err := loadPluginCUE(pluginDir, "mu.cue")
	if err != nil {
		return PluginPackage{}, err
	}
	var mf pluginManifestFile
	if err := manifest.Decode(&mf); err != nil {
		return PluginPackage{}, fmt.Errorf("decode %s: %w", filepath.Join(pluginDir, "mu.cue"), err)
	}

	pkg := PluginPackage{Schemas: mf.Plugin.Schemas}
	if _, err := os.Stat(filepath.Join(pluginDir, "pudl.cue")); os.IsNotExist(err) {
		return pkg, nil
	} else if err != nil {
		return PluginPackage{}, fmt.Errorf("stat pudl.cue: %w", err)
	}
	mappings, err := loadPluginCUE(pluginDir, "pudl.cue")
	if err != nil {
		return PluginPackage{}, err
	}
	var pm pluginMappingsFile
	if err := mappings.Decode(&pm); err != nil {
		return PluginPackage{}, fmt.Errorf("decode %s: %w", filepath.Join(pluginDir, "pudl.cue"), err)
	}
	pkg.Mappings = pm.Mappings
	return pkg, nil
}

// ValidatePluginMappings validates mappings against the active PUDL schema
// namespace before observe records are allowed to use them.
func ValidatePluginMappings(mappings []PluginMapping, graph *inference.InheritanceGraph) error {
	if len(mappings) == 0 {
		return nil
	}
	if graph == nil {
		return fmt.Errorf("plugin mappings require a loaded PUDL schema graph")
	}
	seen := make(map[string]struct{}, len(mappings))
	for i, mapping := range mappings {
		if strings.TrimSpace(mapping.ResourceType) == "" {
			return fmt.Errorf("mappings[%d]: resource_type is required", i)
		}
		if strings.TrimSpace(mapping.Schema) == "" {
			return fmt.Errorf("mappings[%d]: schema is required", i)
		}
		if _, exists := seen[mapping.ResourceType]; exists {
			return fmt.Errorf("mappings: duplicate resource_type %q", mapping.ResourceType)
		}
		seen[mapping.ResourceType] = struct{}{}
		if !strings.HasPrefix(mapping.Schema, "pudl/") {
			return fmt.Errorf("mappings[%d]: schema %q must be in the PUDL namespace", i, mapping.Schema)
		}
		if !graph.HasSchema(mapping.Schema) {
			return fmt.Errorf("mappings[%d]: schema %q is not loaded", i, mapping.Schema)
		}
	}
	return nil
}

// MappingIndex returns the routing map used by ObserveIngest.
func MappingIndex(mappings []PluginMapping) map[string]string {
	if len(mappings) == 0 {
		return nil
	}
	index := make(map[string]string, len(mappings))
	for _, mapping := range mappings {
		index[mapping.ResourceType] = mapping.Schema
	}
	return index
}

// SyncPluginSchemas materializes plugin-owned schemas into PUDL's append-only
// schema cache. PUDL never writes the pudl/... namespace from a plugin package.
func SyncPluginSchemas(cache *muschemas.Cache, pluginDir string) (PluginPackage, error) {
	if cache == nil {
		return PluginPackage{}, fmt.Errorf("schema cache is required")
	}
	pkg, err := LoadPluginPackage(pluginDir)
	if err != nil {
		return PluginPackage{}, err
	}
	if err := syncPluginPackage(cache, pluginDir, pkg); err != nil {
		return PluginPackage{}, err
	}
	return pkg, nil
}

func syncPluginPackage(cache *muschemas.Cache, pluginDir string, pkg PluginPackage) error {
	for _, decl := range pkg.Schemas {
		if strings.HasPrefix(decl.Module, "pudl/") {
			return fmt.Errorf("plugin schema %q uses reserved PUDL namespace", decl.Module)
		}
		files, err := readPluginSchemaFiles(pluginDir, decl.Path)
		if err != nil {
			return fmt.Errorf("schema %s@%s: %w", decl.Module, decl.Version, err)
		}
		if err := cache.Insert(decl.Module, decl.Version, files); err != nil {
			return fmt.Errorf("cache schema %s@%s: %w", decl.Module, decl.Version, err)
		}
	}
	return nil
}

func loadPluginCUE(dir, file string) (cue.Value, error) {
	path := filepath.Join(dir, file)
	if _, err := os.Stat(path); err != nil {
		return cue.Value{}, fmt.Errorf("reading %s: %w", path, err)
	}
	instances := load.Instances([]string{file}, &load.Config{Dir: dir})
	if len(instances) == 0 {
		return cue.Value{}, fmt.Errorf("no CUE instance loaded from %s", path)
	}
	if instances[0].Err != nil {
		return cue.Value{}, fmt.Errorf("loading %s: %w", path, instances[0].Err)
	}
	value := cuecontext.New().BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("building %s: %w", path, err)
	}
	return value, nil
}

func readPluginSchemaFiles(pluginDir, schemaPath string) ([]muschemas.File, error) {
	if schemaPath == "" || filepath.IsAbs(schemaPath) {
		return nil, fmt.Errorf("schema path %q must be relative to the plugin bundle", schemaPath)
	}
	root := filepath.Join(pluginDir, schemaPath)
	if rel, err := filepath.Rel(pluginDir, root); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("schema path %q escapes the plugin bundle", schemaPath)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", schemaPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", schemaPath)
	}
	var files []muschemas.File
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cue") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, muschemas.File{RelPath: filepath.ToSlash(rel), Content: content})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s contains no .cue files", schemaPath)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files, nil
}
