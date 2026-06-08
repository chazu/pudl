package factstore

import (
	"path/filepath"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/workspace"
)

// GlobalDir returns the global pudl config directory (~/.pudl).
func GlobalDir() string {
	return config.GetPudlDir()
}

// Workspace describes a resolved pudl workspace for an external consumer.
type Workspace struct {
	// RepoDir is the absolute path to the repo-scoped .pudl directory, or empty
	// when not inside a workspace (global-only mode).
	RepoDir string

	// GlobalDir is the global pudl directory (~/.pudl).
	GlobalDir string

	// RulePaths is the ordered list of Datalog rule directories to pass to
	// eval.LoadRulesFromPaths, global first then repo, matching `pudl query`.
	// The loader gives later paths priority, so repo rules shadow global rules
	// with the same name.
	RulePaths []string
}

// DiscoverWorkspace resolves the pudl workspace for the given working directory.
// It walks up from cwd looking for a repo workspace and assembles the rule
// search paths the same way the CLI does. RepoDir is empty in global-only mode.
func DiscoverWorkspace(cwd string) (*Workspace, error) {
	globalDir := config.GetPudlDir()

	ws, err := workspace.Discover(cwd)
	if err != nil {
		return nil, err
	}

	out := &Workspace{GlobalDir: globalDir}

	// Global rules first, then repo. The loader prioritises later paths, so
	// repo rules shadow global rules with the same name (matches `pudl query`).
	out.RulePaths = append(out.RulePaths, filepath.Join(globalDir, "schema", "pudl", "rules"))
	if ws != nil {
		out.RepoDir = ws.PudlDir
		out.RulePaths = append(out.RulePaths, filepath.Join(ws.PudlDir, "schema", "pudl", "rules"))
	}

	return out, nil
}
