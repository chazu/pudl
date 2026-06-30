package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/mubridge"
	"github.com/chazu/pudl/internal/systemmodel"
)

// recordModelInstance upserts the run's #SystemModel instance into the catalog
// (schema pudl/systemmodel.#SystemModel, identity = name) so every model that
// has been run is inventoriable via `pudl list`/`query`. It reuses the shipped
// observe ingester — the instance lands as an ordinary catalog record.
func recordModelInstance(m *systemmodel.SystemModel) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil {
		return err
	}
	rec["name"] = m.Name
	rec["_schema"] = "systemmodel.system_model" // -> pudl/systemmodel.#SystemModel

	results := []mubridge.ObserveResult{{
		Target:  modelTarget(m.Name),
		Current: map[string]any{"records": []any{rec}},
	}}
	wrapped, err := json.Marshal(results)
	if err != nil {
		return err
	}
	_, err = ingestObserveOutput(wrapped)
	return err
}

// modelTarget is the catalog `definition` key for a model instance row — the
// same string recordModelInstance ingests under, so run-status writes (and the
// `pudl model list` / `pudl status` reads) all address that one row.
func modelTarget(name string) string { return "//models/" + name }

// modelTargetKey is the catalog `target`-column key for a model's instance row:
// the mu target with its "//" prefix stripped, matching how the observe ingester
// stores it (mubridge normalizeTarget). Status reads/writes MUST use this form —
// keying on modelTarget (with "//") silently misses the row, so run verdicts
// never persist.
func modelTargetKey(name string) string { return strings.TrimPrefix(modelTarget(name), "//") }

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
	return nil, "", "", fmt.Errorf("system model %q not found in %s — register it as a #SystemModel-derived definition", name, strings.Join(searched, ", "))
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
	return match.Model, match.Dir, nil
}

// shortDefName strips the package prefix and leading '#' from a canonical schema
// name: "models.#GithubChazu" -> "GithubChazu".
func shortDefName(canonical string) string {
	if i := strings.LastIndex(canonical, "#"); i >= 0 {
		return canonical[i+1:]
	}
	return canonical
}
