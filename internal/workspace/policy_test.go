package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chazu/pudl/internal/datalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoWorkspace stages a repo with a .pudl workspace, returning the repo root.
func repoWorkspace(t *testing.T, parent, name string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	pudlDir := filepath.Join(root, ".pudl")
	require.NoError(t, os.MkdirAll(filepath.Join(pudlDir, "schema", "pudl", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "workspace.cue"),
		[]byte("name: \""+name+"\"\n"), 0o644))
	return root
}

func globalDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "global-pudl")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "schema", "pudl", "rules"), 0o755))
	return dir
}

func TestPolicy_GlobalOnly(t *testing.T) {
	global := globalDir(t)
	// A directory with no .pudl anywhere above it inside the temp tree.
	cwd := t.TempDir()

	policy, err := Resolve(cwd, global)
	require.NoError(t, err)

	if policy.InWorkspace() {
		t.Skip("the temp dir sits inside a real pudl workspace; global-only case not reproducible here")
	}
	assert.Equal(t, ModeGlobal, policy.Mode)
	assert.Equal(t, "global", policy.EffectiveOrigin)
	assert.Equal(t, "global", policy.CatalogScope())
	assert.Equal(t, []string{filepath.Join(global, "schema")}, policy.SchemaSearchPaths)
	assert.Equal(t, []string{filepath.Join(global, "schema")}, policy.ModelSearchPaths)
	assert.Equal(t, []string{filepath.Join(global, "schema", "pudl", "rules")}, policy.RuleSearchPaths)

	_, err = policy.RuleWritePath()
	assert.Error(t, err, "there is no repo-scoped place to write a rule outside a workspace")
}

func TestPolicy_LocalWorkspacePrecedesGlobal(t *testing.T) {
	global := globalDir(t)
	root := repoWorkspace(t, t.TempDir(), "myrepo")

	policy, err := Resolve(root, global)
	require.NoError(t, err)

	require.True(t, policy.InWorkspace())
	assert.Equal(t, ModeWorkspace, policy.Mode)
	assert.Equal(t, "myrepo", policy.EffectiveOrigin)
	assert.Equal(t, "myrepo", policy.CatalogScope())

	pudlDir := filepath.Join(root, ".pudl")
	assert.Equal(t, []string{
		filepath.Join(pudlDir, "schema"),
		filepath.Join(global, "schema"),
	}, policy.SchemaSearchPaths, "repo first: these lists are searched front-to-back")
	assert.Equal(t, []string{
		filepath.Join(pudlDir, "definitions"),
		filepath.Join(global, "schema", "definitions"),
	}, policy.DefinitionSearchPaths)
	assert.Equal(t, []string{
		filepath.Join(pudlDir, "schema"),
		filepath.Join(global, "schema"),
	}, policy.ModelSearchPaths)
	// Populators are owner-relative, not repo-then-global: a model registered
	// globally must not pick up a repo's populator of the same name.
	assert.Equal(t, []string{
		filepath.Join(pudlDir, "populators"),
		pudlDir,
		"/models",
	}, policy.PopulatorPathsFor(pudlDir, "/models"))
	assert.Empty(t, policy.PopulatorPathsFor("", ""))

	// Rules run the other way round: the loader walks its arguments in reverse
	// and keeps the first name it sees, so *later* wins. Repo still shadows
	// global; only the spelling differs.
	assert.Equal(t, []string{
		filepath.Join(global, "schema", "pudl", "rules"),
		filepath.Join(pudlDir, "schema", "pudl", "rules"),
	}, policy.RuleSearchPaths)

	write, err := policy.RuleWritePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(pudlDir, "schema", "pudl", "rules"), write)
}

func TestPolicy_SecretsWritablePolicyPreservesAbsentVersusDenyAll(t *testing.T) {
	global := globalDir(t)
	root := repoWorkspace(t, t.TempDir(), "locked")
	workspaceCue := filepath.Join(root, ".pudl", "workspace.cue")
	require.NoError(t, os.WriteFile(workspaceCue, []byte("name: \"locked\"\nsecrets: writable_refs: []\n"), 0o644))

	policy, err := Resolve(root, global)
	require.NoError(t, err)
	refs, configured := policy.SecretsWritablePolicy()
	assert.True(t, configured)
	assert.Empty(t, refs)

	globalPolicy, err := Resolve(t.TempDir(), global)
	require.NoError(t, err)
	if globalPolicy.InWorkspace() {
		t.Skip("the temp dir sits inside a real pudl workspace")
	}
	refs, configured = globalPolicy.SecretsWritablePolicy()
	assert.False(t, configured)
	assert.Nil(t, refs)
}

func TestPolicy_RepoRulesShadowGlobalRules(t *testing.T) {
	// The shadowing contract, asserted through the loader rather than by reading
	// the order off the slice: a rule name defined in both resolves to the repo's.
	global := globalDir(t)
	root := repoWorkspace(t, t.TempDir(), "myrepo")

	require.NoError(t, os.WriteFile(
		filepath.Join(global, "schema", "pudl", "rules", "r.cue"),
		[]byte("package rules\n\nshared: {\n\thead: {rel: \"shared\", args: {x: \"$X\"}}\n\tbody: [{rel: \"global_source\", args: {x: \"$X\"}}]\n}\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".pudl", "schema", "pudl", "rules", "r.cue"),
		[]byte("package rules\n\nshared: {\n\thead: {rel: \"shared\", args: {x: \"$X\"}}\n\tbody: [{rel: \"repo_source\", args: {x: \"$X\"}}]\n}\n"), 0o644))

	policy, err := Resolve(root, global)
	require.NoError(t, err)

	rules, err := loadRulesForTest(policy.RuleSearchPaths)
	require.NoError(t, err)
	require.Len(t, rules, 1, "the shadowed rule appears once")
	assert.Equal(t, "repo_source", rules[0], "the repo definition wins")
}

func TestPolicy_NestedWorkspaceWinsOutright(t *testing.T) {
	global := globalDir(t)
	outer := repoWorkspace(t, t.TempDir(), "outer")
	inner := repoWorkspace(t, outer, "inner")

	policy, err := Resolve(inner, global)
	require.NoError(t, err)

	require.True(t, policy.InWorkspace())
	assert.Equal(t, "inner", policy.EffectiveOrigin)
	assert.Equal(t, filepath.Join(inner, ".pudl"), policy.Workspace.PudlDir,
		"the innermost workspace wins outright; the outer one contributes nothing")
	assert.NotContains(t, policy.SchemaSearchPaths, filepath.Join(outer, ".pudl", "schema"))
}

func TestPolicy_RulePathsForModel(t *testing.T) {
	global := globalDir(t)
	root := repoWorkspace(t, t.TempDir(), "myrepo")
	modelDir := t.TempDir()

	policy, err := Resolve(root, global)
	require.NoError(t, err)

	// [global, modelDir, repo] — a repo workspace's rules shadow the model's own.
	// Arguably backwards, preserved verbatim; see the design note.
	assert.Equal(t, []string{
		filepath.Join(global, "schema", "pudl", "rules"),
		filepath.Join(modelDir, "rules"),
		filepath.Join(root, ".pudl", "schema", "pudl", "rules"),
	}, policy.RulePathsForModel(modelDir))

	assert.Equal(t, policy.RuleSearchPaths, policy.RulePathsForModel(""),
		"no model dir, no splice")
}

func TestPolicy_RulePathsForModelOutsideAWorkspace(t *testing.T) {
	global := globalDir(t)
	cwd := t.TempDir()
	modelDir := t.TempDir()

	policy, err := Resolve(cwd, global)
	require.NoError(t, err)
	if policy.InWorkspace() {
		t.Skip("the temp dir sits inside a real pudl workspace")
	}

	assert.Equal(t, []string{
		filepath.Join(global, "schema", "pudl", "rules"),
		filepath.Join(modelDir, "rules"),
	}, policy.RulePathsForModel(modelDir))
}

func TestPolicy_SearchOrderIsNotFilteredToWhatExists(t *testing.T) {
	// RuleSearchPaths is an order, not a snapshot of what happens to be on disk.
	// A caller reporting where rules are looked for needs the whole list; the
	// loader is what tolerates a path with nothing in it.
	global := globalDir(t)
	root := repoWorkspace(t, t.TempDir(), "myrepo")
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".pudl", "schema", "pudl", "rules")))

	policy, err := Resolve(root, global)
	require.NoError(t, err)
	assert.Len(t, policy.RuleSearchPaths, 2)
}

// loadRulesForTest loads rules through the real loader and returns each rule's
// first body relation, which is what distinguishes the global and repo
// definitions in the shadowing test.
func loadRulesForTest(paths []string) ([]string, error) {
	rules, err := datalog.LoadRulesFromPaths(paths...)
	if err != nil {
		return nil, err
	}
	var sources []string
	for _, rule := range rules {
		if len(rule.Body) > 0 {
			sources = append(sources, rule.Body[0].Rel)
		}
	}
	return sources, nil
}
