package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphexport"
)

// The fixtures' repositories as graph nodes: type and name.
const (
	deployRepoNode = "argocd.repository:git.example.com/example-org/example-deploy"
	chartsRepoNode = "argocd.repository:charts.example.com/stable"
)

// TestArgoCDPackFixtureRepositories runs the repository fixture collector:
// one record per repository the fixtures name. edge-proxy names the
// manifests repository in the scp form (git@host:org/repo.git), the
// regional ApplicationSet's template in the ssh form with an upper-case
// host, the rest in the https form: one record. Only the git repository
// holds a git.repo key.
func TestArgoCDPackFixtureRepositories(t *testing.T) {
	argocdWorkspace(t, "argocd-fixture-repositories")

	repos := map[string]map[string]any{}
	for _, rec := range storedRecords(t, "argocd.#Repository") {
		repos[rec["url"].(string)] = rec
	}
	require.Equal(t, map[string]map[string]any{
		deployRepoID: {
			"url": deployRepoID, "name": "git.example.com/example-org/example-deploy",
			"type": "git", "gitURL": deployRepoID,
		},
		"https://charts.example.com/stable": {
			"url": "https://charts.example.com/stable", "name": "charts.example.com/stable", "type": "helm",
		},
	}, repos)

	var keys []string
	for _, args := range queryArgs(t, "same_resource_key") {
		if args["kind"] == "git.repo" {
			keys = append(keys, args["key"].(string))
		}
	}
	require.Equal(t, []string{`{"url":"` + deployRepoID + `"}`}, keys)
}

// sourcedFromModels are the graph models with the repository records.
var sourcedFromModels = append(append([]string{}, argocdGraphModels...), "argocd-fixture-repositories")

// TestArgoCDPackSourcedFrom checks sourced_from: from each Application to
// the repositories its sources name (single source, multi-source), and
// from each ApplicationSet to the repositories its template sources and
// git generators name, inside a matrix or merge too. The scp and ssh forms
// reach the same node as the https form.
func TestArgoCDPackSourcedFrom(t *testing.T) {
	argocdWorkspace(t, sourcedFromModels...)
	g := exportGraph(t)
	requireCompleteVocabulary(t, g)

	const app, set = "argocd.application:", "argocd.applicationset:"
	require.ElementsMatch(t, []string{
		app + "cluster-addons -> " + chartsRepoNode,
		app + "shop-checkout -> " + deployRepoNode,
		app + "shop-checkout -> " + chartsRepoNode,
		app + "shop-catalog -> " + deployRepoNode,
		app + "edge-proxy -> " + deployRepoNode,
		app + "hostile-app -> " + deployRepoNode,
		app + "hostile-app -> " + chartsRepoNode,
		app + "hostile-single -> " + deployRepoNode,
		set + "shop -> " + deployRepoNode,
		set + "platform -> " + chartsRepoNode,
		set + "regional -> " + deployRepoNode,
		set + "hostile-set -> " + deployRepoNode,
		set + "hostile-set -> " + chartsRepoNode,
	}, argocdEdges(g, "sourced_from"))

	props := sourcedFromProperties(g)
	require.Equal(t, map[string]any{"path": "edge/proxy", "target_revision": "main"},
		props[app+"edge-proxy -> "+deployRepoNode], "the scp form's source")
	require.Equal(t, map[string]any{"path": "regional", "target_revision": "release", "revision": "release"},
		props[set+"regional -> "+deployRepoNode], "the ssh template source and the merge's git generator")
	require.Equal(t, map[string]any{"chart": "{{.component}}", "target_revision": "4.2.0"},
		props[set+"platform -> "+chartsRepoNode])
}

// TestArgoCDPackSourcedFromNeedsARecord checks that no rule draws a
// repository node: without the repository records there is no
// sourced_from edge, no repository node and no unresolved reference.
func TestArgoCDPackSourcedFromNeedsARecord(t *testing.T) {
	argocdWorkspace(t, argocdGraphModels...)
	g := exportGraph(t)

	require.Empty(t, argocdEdges(g, "sourced_from"))
	for _, r := range g.Resources {
		require.NotEqual(t, "argocd.repository", r.Type)
		require.NotContains(t, r.ID, "example-deploy", "no node for the repository")
	}
	for _, rel := range g.Relationships {
		for _, end := range []string{rel.Source, rel.Target} {
			if strings.HasPrefix(end, graphexport.RefPrefix) {
				require.NotContains(t, end, "example-deploy", "unresolved %s", rel.ID)
				require.NotContains(t, end, "charts.example.com", "unresolved %s", rel.ID)
			}
		}
	}
}

// sourcedFromProperties keys each sourced_from edge's properties by
// "<type>:<name> -> <type>:<name>".
func sourcedFromProperties(g graphexport.Graph) map[string]map[string]any {
	names := map[string]string{}
	for _, r := range g.Resources {
		names[r.ID] = r.Type + ":" + r.Name
	}
	out := map[string]map[string]any{}
	for _, rel := range g.Relationships {
		if rel.Type == "sourced_from" {
			out[names[rel.Source]+" -> "+names[rel.Target]] = rel.Properties
		}
	}
	return out
}
