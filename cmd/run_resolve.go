package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/systemmodel"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/workspace"
)

// resolveModel finds a registered #SystemModel-derived schema by name. It
// searches the project-level .pudl/schema first (if a workspace is found by
// walking up from the cwd), then the global ~/.pudl/schema — project wins. A
// model is any definition whose inherited _pudl.resource_type is "system_model"
// and that decodes to a concrete instance whose `name` (or short definition
// name) matches. Returns the decoded model and the directory it was loaded from
// (the base for resolving eweSource + relative plugin paths).
func resolveModel(name string) (*systemmodel.SystemModel, string, error) {
	var dirs []string
	if ws, _ := workspace.Discover("."); ws != nil && ws.SchemaPath != "" {
		dirs = append(dirs, ws.SchemaPath)
	}
	dirs = append(dirs, filepath.Join(config.GetPudlDir(), "schema"))

	var searched []string
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		searched = append(searched, dir)
		m, modelDir, err := resolveModelIn(dir, name)
		if err != nil {
			return nil, "", err
		}
		if m != nil {
			return m, modelDir, nil
		}
	}
	if len(searched) == 0 {
		return nil, "", fmt.Errorf("system model %q not found: no schema repository (run `pudl init`)", name)
	}
	return nil, "", fmt.Errorf("system model %q not found in %s — register it as a #SystemModel-derived definition", name, strings.Join(searched, ", "))
}

// resolveModelIn searches one schema directory for a system-model definition
// matching name. Returns (nil, "", nil) if none matched here.
func resolveModelIn(dir, name string) (*systemmodel.SystemModel, string, error) {
	loader := validator.NewCUEModuleLoader(dir)
	modules, err := loader.LoadAllModules()
	if err != nil {
		return nil, "", fmt.Errorf("load schemas in %s: %w", dir, err)
	}

	var matchModel *systemmodel.SystemModel
	var matchDir, matchName string
	for _, mod := range modules {
		for schemaName, meta := range mod.Metadata {
			if meta.ResourceType != "system_model" {
				continue
			}
			val, ok := mod.Schemas[schemaName]
			if !ok {
				continue
			}
			m, derr := systemmodel.DecodeValue(val)
			if derr != nil {
				// The abstract base #SystemModel (no concrete name) and any
				// incomplete def land here — skip, they aren't runnable models.
				continue
			}
			if m.Name != name && shortDefName(schemaName) != name {
				continue
			}
			if matchModel != nil && matchName != schemaName {
				return nil, "", fmt.Errorf("system model %q is ambiguous in %s (matches %s and %s)", name, dir, matchName, schemaName)
			}
			matchModel, matchDir, matchName = m, mod.LoadPath, schemaName
		}
	}
	return matchModel, matchDir, nil
}

// shortDefName strips the package prefix and leading '#' from a canonical schema
// name: "models.#GithubChazu" -> "GithubChazu".
func shortDefName(canonical string) string {
	if i := strings.LastIndex(canonical, "#"); i >= 0 {
		return canonical[i+1:]
	}
	return canonical
}
