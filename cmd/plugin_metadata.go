package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/mubridge"
	"github.com/chazu/pudl/internal/muschemas"
	"github.com/chazu/pudl/internal/systemmodel"
)

type observePluginRef struct {
	LocalDir    string
	ProjectRoot string
	Name        string
	Digest      string
}

// loadObservePluginMetadata keeps the standalone local-plugin API used by
// `pudl mu ingest-observe` and tests.
func loadObservePluginMetadata(pluginDir string, graph *inference.InheritanceGraph) (map[string]string, error) {
	return loadObservePluginMetadataFor(observePluginRef{LocalDir: pluginDir}, graph)
}

func loadObservePluginMetadataFor(source observePluginRef, graph *inference.InheritanceGraph) (map[string]string, error) {
	pluginDir := source.LocalDir
	var pkg mubridge.PluginPackage
	var err error
	if pluginDir == "" && source.Digest != "" {
		cache, cacheErr := muschemas.New(SchemaCacheRoot())
		if cacheErr != nil {
			return nil, fmt.Errorf("open plugin schema cache: %w", cacheErr)
		}
		pkg, err = mubridge.SyncInstalledPluginSchemas(cache, source.ProjectRoot, source.Name, source.Digest)
		if err != nil {
			return nil, fmt.Errorf("sync installed plugin %q schemas: %w", source.Name, err)
		}
		if err := mubridge.ValidatePluginMappings(pkg.Mappings, graph); err != nil {
			return nil, fmt.Errorf("validate plugin PUDL mappings: %w", err)
		}
		return mubridge.MappingIndex(pkg.Mappings), nil
	}
	if pluginDir == "" {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "mu.cue")); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("inspect plugin manifest: %w", err)
	}
	cache, err := muschemas.New(SchemaCacheRoot())
	if err != nil {
		return nil, fmt.Errorf("open plugin schema cache: %w", err)
	}
	pkg, err = mubridge.SyncPluginSchemas(cache, pluginDir)
	if err != nil {
		return nil, fmt.Errorf("sync plugin schemas: %w", err)
	}
	if err := mubridge.ValidatePluginMappings(pkg.Mappings, graph); err != nil {
		return nil, fmt.Errorf("validate plugin PUDL mappings: %w", err)
	}
	return mubridge.MappingIndex(pkg.Mappings), nil
}

func findMuLockRoot(start string) string {
	if strings.TrimSpace(start) == "" {
		return ""
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "mu.lock")); err == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolveObservePluginSource(m *systemmodel.SystemModel, modelDir, muRoot string) observePluginRef {
	if m == nil || m.Populate.Kind() != systemmodel.KindPluginObserve {
		return observePluginRef{}
	}
	def, ok := m.PluginByName(m.Populate.Plugin)
	if !ok {
		return observePluginRef{Name: m.Populate.Plugin}
	}
	source := observePluginRef{Name: def.Name, Digest: def.Digest}
	if def.Script != "" {
		script := def.Script
		if !filepath.IsAbs(script) {
			script = filepath.Join(modelDir, script)
		}
		source.LocalDir = filepath.Dir(script)
		return source
	}
	for _, candidate := range []string{muRoot, modelDir, mustCurrentDir()} {
		if source.ProjectRoot = findMuLockRoot(candidate); source.ProjectRoot != "" {
			break
		}
	}
	return source
}

func ensureObservePluginAvailable(source observePluginRef) error {
	if source.LocalDir != "" || source.Digest == "" {
		return nil
	}
	_, _, err := mubridge.LoadInstalledPluginPackage(source.ProjectRoot, source.Name, source.Digest)
	if err != nil {
		return fmt.Errorf("prepare plugin %q: %w", source.Name, err)
	}
	return nil
}
