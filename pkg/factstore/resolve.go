package factstore

import (
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
	// with the same name. Directories that do not exist are omitted.
	RulePaths []string
}

// DiscoverWorkspace resolves the pudl workspace for the given working directory.
// It walks up from cwd looking for a repo workspace and assembles the rule
// search paths the same way the CLI does. RepoDir is empty in global-only mode.
//
// "The same way" is now by construction rather than by coincidence: both this
// and the CLI read one workspace.Policy. This function previously spelled out
// the global-then-repo ordering itself, one of four independent copies.
func DiscoverWorkspace(cwd string) (*Workspace, error) {
	globalDir := config.GetPudlDir()

	policy, err := workspace.Resolve(cwd, globalDir)
	if err != nil {
		return nil, err
	}

	out := &Workspace{
		GlobalDir: globalDir,
		RulePaths: policy.RuleSearchPaths,
	}
	if policy.InWorkspace() {
		out.RepoDir = policy.Workspace.PudlDir
	}
	return out, nil
}
