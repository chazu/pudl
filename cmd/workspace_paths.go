package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/chazu/pudl/internal/config"
)

// effectivePudlDir is the one persistence root for the current invocation.
// Inside a repository workspace, catalog rows, raw data, reports, approvals,
// snapshots, and other durable state all belong under that repository's
// .pudl directory. Outside one, the global ~/.pudl root remains the fallback.
func effectivePudlDir() string {
	if wsPolicy != nil && wsPolicy.InWorkspace() && wsPolicy.Workspace.PudlDir != "" {
		return wsPolicy.Workspace.PudlDir
	}
	return config.GetPudlDir()
}

// loadEffectiveConfig loads configuration from the active persistence root.
// Repository schema/data paths are fixed beneath .pudl: accepting an edited
// path that escapes the root would silently defeat workspace isolation.
func loadEffectiveConfig() (*config.Config, error) {
	root := effectivePudlDir()
	cfg, err := config.LoadFrom(root)
	if err != nil {
		return nil, err
	}
	if wsPolicy != nil && wsPolicy.InWorkspace() {
		wantSchema := filepath.Join(root, "schema")
		wantData := filepath.Join(root, "data")
		if filepath.Clean(cfg.SchemaPath) != wantSchema || filepath.Clean(cfg.DataPath) != wantData {
			return nil, fmt.Errorf("repository workspace paths must stay under %s (schema_path=%s, data_path=%s)", root, wantSchema, wantData)
		}
	}
	return cfg, nil
}

// effectiveSchemaPaths returns the workspace-first schema search order. The
// workspace context is authoritative when available; the config path remains
// a safe fallback for tests and commands invoked without Cobra lifecycle hooks.
func effectiveSchemaPaths(cfg *config.Config) []string {
	var paths []string
	if wsPolicy != nil {
		paths = append(paths, wsPolicy.SchemaSearchPaths...)
	}
	if len(paths) == 0 && cfg != nil && cfg.SchemaPath != "" {
		paths = append(paths, cfg.SchemaPath)
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		path, _ = filepath.Abs(path)
		if !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}

func effectiveSchemaPath(cfg *config.Config) string {
	paths := effectiveSchemaPaths(cfg)
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// queryRulePaths is the rule search order for `pudl query`, taken from the
// run's one workspace policy. configDir is honoured as a fallback for callers
// invoked without the Cobra lifecycle (tests), which is the same escape hatch
// effectiveSchemaPaths keeps.
func queryRulePaths(configDir string) []string {
	if wsPolicy != nil {
		return wsPolicy.RuleSearchPaths
	}
	return []string{filepath.Join(configDir, "schema", "pudl", "rules")}
}

// rulePathsForModel is the rule search order for a model's checks.
//
// The fallback mirrors what the policy would produce minus the repo workspace,
// for callers invoked without the Cobra lifecycle (tests). It is spelled out
// rather than resolved on demand: resolving would make a test's answer depend on
// whether it happens to run inside a repo that has a .pudl directory.
func rulePathsForModel(modelDir string) []string {
	if wsPolicy != nil {
		return wsPolicy.RulePathsForModel(modelDir)
	}
	candidates := []string{filepath.Join(config.GetPudlDir(), "schema", "pudl", "rules")}
	if modelDir != "" {
		candidates = append(candidates, filepath.Join(modelDir, "rules"))
	}
	return candidates
}
