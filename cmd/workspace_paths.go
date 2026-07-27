package cmd

import (
	"path/filepath"

	"github.com/chazu/pudl/internal/config"
)

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
