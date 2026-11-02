package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// resolveModel finds a registered #SystemModel-derived schema by name. It
// searches the project-level .pudl/schema first (if a workspace is found by
// walking up from the cwd), then the global ~/.pudl/schema — project wins. A
// model is any definition whose inherited _pudl.resource_type is "system_model"
// and that decodes to a concrete instance whose `name` (or short definition
// name) matches. Returns the decoded model and the directory it was loaded from
// (the base for resolving eweSource + relative plugin paths).
// resolveModel returns the decoded model, the directory its schema was loaded
// from (modelDir), and the pudl root that owns it (the .pudl dir, parent of the
// schema dir) — the base for resolving a populators/ path.
func resolveModel(name string) (m *systemmodel.SystemModel, modelDir, pudlRoot string, err error) {
	var searched []string
	for _, dir := range modelSearchDirs() {
		if st, statErr := os.Stat(dir); statErr != nil || !st.IsDir() {
			continue
		}
		searched = append(searched, dir)
		found, md, rerr := resolveModelIn(dir, name)
		if rerr != nil {
			return nil, "", "", rerr
		}
		if found != nil {
			return found, md, filepath.Dir(dir), nil // pudlRoot = parent of schema dir
		}
	}
	if len(searched) == 0 {
		return nil, "", "", fmt.Errorf("system model %q not found: no schema repository (run `pudl init`)", name)
	}
	return nil, "", "", fmt.Errorf("system model %q not found in %s — register it locally or vendor its definition package in .pudl/workspace.cue (inspect: pudl config --legacy-dependencies)", name, strings.Join(searched, ", "))
}

// resolveModelTemplate resolves a registered model from the invocation's
// model search dirs (see systemmodel.ResolveTemplate).
func resolveModelTemplate(name string) (*systemmodel.ModelTemplate, error) {
	return systemmodel.ResolveTemplate(modelSearchDirs(), name)
}

// resolveModelIn searches one schema directory for a system-model definition
// matching name (by instance `name:` or short definition name). Returns
// (nil, "", nil) if none matched here. Shares the per-dir iterator with
// listModelsIn (model_list.go).
func resolveModelIn(dir, name string) (*systemmodel.SystemModel, string, error) {
	found, err := listModelsIn(dir)
	if err != nil {
		return nil, "", err
	}
	var match *ModelInfo
	for i := range found {
		mi := &found[i]
		if mi.Name != name && mi.DefName != name {
			continue
		}
		if match != nil && match.SchemaName != mi.SchemaName {
			return nil, "", fmt.Errorf("system model %q is ambiguous in %s (matches %s and %s)", name, dir, match.SchemaName, mi.SchemaName)
		}
		match = mi
	}
	if match == nil {
		return nil, "", nil
	}
	if match.Model == nil {
		_, err := match.Template.Model()
		return nil, "", fmt.Errorf("system model %q is invalid: %w", name, err)
	}
	return match.Model, match.Dir, nil
}
