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
	if wsCtx != nil {
		paths = append(paths, wsCtx.SchemaSearchPaths...)
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
