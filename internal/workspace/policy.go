package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chazu/pudl/internal/config"
)

// Mode is whether this invocation is running inside a repo workspace.
type Mode string

const (
	ModeWorkspace Mode = "workspace"
	ModeGlobal    Mode = "global"
)

// Policy is every path decision an invocation makes, resolved once.
//
// Workspace precedence was implemented but re-implemented per call site: four
// independent assemblies of the same "global then repo" rule for Datalog rules
// alone (`pudl query`, model checks, the public API, and the rule-write target),
// plus a second `workspace.Discover` for model search dirs that could in
// principle disagree with the one resolved at startup. The report's success
// measure — workspace resolution identical across CLI and library — held by
// coincidence rather than by construction.
type Policy struct {
	// Workspace is non-nil when running inside a per-repo workspace.
	Workspace *Workspace

	// GlobalDir is always ~/.pudl/
	GlobalDir string

	// Mode records which regime produced these paths.
	Mode Mode

	// EffectiveOrigin is the workspace name, or "global" outside one. It is the
	// origin imports and runs are recorded under.
	EffectiveOrigin string

	// SchemaSearchPaths is searched front-to-back: repo first, then global.
	SchemaSearchPaths []string

	// DefinitionSearchPaths is searched front-to-back: repo first, then global.
	DefinitionSearchPaths []string

	// RuleSearchPaths is ordered global first, then repo — the *opposite*
	// direction, because LoadRulesFromPaths walks its arguments in reverse and
	// skips already-seen rule names, so later paths win. Repo rules shadow global
	// ones either way; only the spelling differs.
	//
	// This is the search *order*, not the subset that happens to exist right now:
	// a caller reporting where rules are looked for needs the whole list, and the
	// loader is what tolerates a path that is missing or is not a directory.
	RuleSearchPaths []string

	// ModelSearchPaths is where #SystemModel definitions are resolved from:
	// repo first, then global.
	ModelSearchPaths []string
}

// Resolve discovers the workspace for a working directory and assembles every
// search path from it.
func Resolve(cwd, globalDir string) (*Policy, error) {
	ws, err := Discover(cwd)
	if err != nil {
		return nil, err
	}
	return build(ws, globalDir), nil
}

// ResolveForCWD resolves the policy for the process's working directory and the
// configured global pudl dir.
func ResolveForCWD() (*Policy, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Resolve(cwd, config.GetPudlDir())
}

// build assembles a policy from a (possibly nil) workspace and a global dir.
func build(ws *Workspace, globalDir string) *Policy {
	globalSchema := filepath.Join(globalDir, "schema")
	globalRules := filepath.Join(globalSchema, "pudl", "rules")

	policy := &Policy{
		Workspace:       ws,
		GlobalDir:       globalDir,
		Mode:            ModeGlobal,
		EffectiveOrigin: string(ModeGlobal),
	}

	if ws != nil {
		policy.Mode = ModeWorkspace
		policy.EffectiveOrigin = ws.Name
		policy.SchemaSearchPaths = append(policy.SchemaSearchPaths, ws.SchemaPath)
		policy.DefinitionSearchPaths = append(policy.DefinitionSearchPaths, ws.DefinitionsPath)
		policy.ModelSearchPaths = append(policy.ModelSearchPaths, ws.SchemaPath)
	}

	policy.SchemaSearchPaths = append(policy.SchemaSearchPaths, globalSchema)
	policy.DefinitionSearchPaths = append(policy.DefinitionSearchPaths,
		filepath.Join(globalSchema, "definitions"))
	policy.ModelSearchPaths = append(policy.ModelSearchPaths, globalSchema)

	rulePaths := []string{globalRules}
	if ws != nil {
		rulePaths = append(rulePaths, filepath.Join(ws.PudlDir, "schema", "pudl", "rules"))
	}
	policy.RuleSearchPaths = rulePaths

	return policy
}

// InWorkspace reports whether a repo workspace was found.
func (p *Policy) InWorkspace() bool { return p != nil && p.Workspace != nil }

// CatalogScope is the catalog origin this workspace implies.
//
// It is an alias for EffectiveOrigin, and deliberately so: the only catalog
// scope a *workspace* implies is the origin its imports and runs are recorded
// under. A run's `--catalog-scope` is not workspace policy — it names a specific
// snapshot or origin per invocation, and Defect 3 made it explicitly required
// rather than inferred, because inferring it let a replay compare against every
// observation in the catalog.
func (p *Policy) CatalogScope() string {
	if p == nil {
		return string(ModeGlobal)
	}
	return p.EffectiveOrigin
}

// RulePathsForModel is RuleSearchPaths with a model's own rules/ directory
// spliced in.
//
// The order is [global, modelDir, repo], which means a repo workspace's rules
// shadow the model's own. That is arguably backwards — a model's rules/ is more
// specific than the repo it sits in — but it is the precedence model checks have
// always had, and changing which rules shadow which changes what a check
// evaluates. Preserved verbatim; see docs/design/2026-07-27-workspace-policy.md.
func (p *Policy) RulePathsForModel(modelDir string) []string {
	if p == nil {
		return nil
	}
	if modelDir == "" {
		return append([]string{}, p.RuleSearchPaths...)
	}

	modelRules := filepath.Join(modelDir, "rules")
	paths := make([]string, 0, len(p.RuleSearchPaths)+1)
	inserted := false
	for _, path := range p.RuleSearchPaths {
		// Splice before the repo path, which is last when present.
		if !inserted && p.InWorkspace() && path == p.repoRulePath() {
			paths = append(paths, modelRules)
			inserted = true
		}
		paths = append(paths, path)
	}
	if !inserted {
		paths = append(paths, modelRules)
	}
	return paths
}

// PopulatorPathsFor is where an #EweTarget's eweSource is looked for, in order.
//
// Deliberately *not* a flat repo-then-global list like the others: a populator
// is resolved relative to the pudl root that owns the model, so a model
// registered globally cannot silently pick up a repo's populator of the same
// name, and vice versa. ownerRoot is that root; modelDir is the co-located
// fallback.
func (p *Policy) PopulatorPathsFor(ownerRoot, modelDir string) []string {
	var paths []string
	if ownerRoot != "" {
		paths = append(paths, filepath.Join(ownerRoot, "populators"), ownerRoot)
	}
	if modelDir != "" {
		paths = append(paths, modelDir)
	}
	return paths
}

// RuleWritePath is where `pudl rule add` puts a new rule: the repo workspace's
// rules directory. Outside a workspace there is no repo-scoped place for it.
func (p *Policy) RuleWritePath() (string, error) {
	if !p.InWorkspace() {
		return "", fmt.Errorf("no repo workspace found (run `pudl workspace init`)")
	}
	return filepath.Join(p.Workspace.PudlDir, "schema", "pudl", "rules"), nil
}

func (p *Policy) repoRulePath() string {
	if !p.InWorkspace() {
		return ""
	}
	return filepath.Join(p.Workspace.PudlDir, "schema", "pudl", "rules")
}
