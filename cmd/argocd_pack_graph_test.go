package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphexport"
)

// argocdGraphModels collect what the Argo CD rules join: the GKE clusters
// and their deployments (GCP pack), the kube context's cluster record
// (Kubernetes pack, prod-private) and the Argo CD objects, collected from
// batch-private.
var argocdGraphModels = []string{
	"gcp-fixture-gke-clusters", "gcp-fixture-k8s-deployments", "k8s-fixture-cluster",
	"argocd-fixture-applications", "argocd-fixture-applicationsets",
}

// appNames maps each Application's resource_id to its name.
func appNames(t *testing.T) map[string]string {
	t.Helper()
	names := map[string]string{}
	for _, args := range queryArgs(t, "argocd_app") {
		names[args["resource_id"].(string)] = args["name"].(string)
	}
	return names
}

// TestArgoCDPackTargetHost checks argocd_app_target_host for both
// destination forms: the in-cluster server resolves to the cluster the
// record came from, a server URL to its host; a named destination has a
// destination fact and no host.
func TestArgoCDPackTargetHost(t *testing.T) {
	argocdWorkspace(t, "argocd-fixture-applications")
	names := appNames(t)

	hosts := map[string]string{}
	for _, args := range queryArgs(t, "argocd_app_target_host") {
		hosts[names[args["resource_id"].(string)]] = args["host"].(string)
	}
	require.Equal(t, map[string]string{
		"cluster-addons": argocdFixtureCluster,
		"hostile-app":    argocdFixtureCluster,
		"shop-checkout":  "198.51.100.30",
		"shop-catalog":   "198.51.100.30",
	}, hosts)

	named := argsWhere(queryArgs(t, "argocd_app_destination"), "name", "prod-private")
	require.Len(t, named, 1)
	require.Equal(t, "edge-proxy", names[named[0]["resource_id"].(string)])
	require.Equal(t, "", named[0]["server"])
	require.Equal(t, "edge", named[0]["namespace"])
}

// argocdEdges lists "<type>:<name> -> <type>:<name>" for the
// relationships of a type.
func argocdEdges(g graphexport.Graph, typ string) []string {
	names := map[string]string{}
	for _, r := range g.Resources {
		names[r.ID] = r.Type + ":" + r.Name
	}
	label := func(id string) string {
		if n, ok := names[id]; ok {
			return n
		}
		return id
	}
	var out []string
	for _, rel := range g.Relationships {
		if rel.Type == typ {
			out = append(out, label(rel.Source)+" -> "+label(rel.Target))
		}
	}
	return out
}

// TestArgoCDPackEdges collects the GCP, Kubernetes and Argo CD fixtures and
// checks the pack's edges. Argo CD runs in batch-private and deploys to
// prod-private by its server URL, as a central Argo CD does: deploys_to
// reaches another cluster record than the one the Application lives in.
// The Application an ApplicationSet generated (shop-catalog) has
// generated_by, deploys_to and deployed_by end to end.
func TestArgoCDPackEdges(t *testing.T) {
	argocdWorkspace(t, argocdGraphModels...)
	g := exportGraph(t)
	requireCompleteVocabulary(t, g)

	const (
		batch = "gcp.container.cluster:batch-private"
		prod  = "gcp.container.cluster:prod-private"
	)
	// prod-private is one node for its GKE record and the kube context's
	// cluster record.
	found := 0
	for _, r := range g.Resources {
		if r.Type+":"+r.Name == prod {
			found++
			var schemas []string
			for _, rep := range r.Representations {
				schemas = append(schemas, rep.Schema)
			}
			require.ElementsMatch(t, []string{"gcp.#GKECluster", "k8s.#Cluster"}, schemas)
		}
	}
	require.Equal(t, 1, found)
	require.ElementsMatch(t, []string{
		"argocd.application:cluster-addons -> " + batch,
		"argocd.application:hostile-app -> " + batch,
		"argocd.application:shop-checkout -> " + prod,
		"argocd.application:shop-catalog -> " + prod,
	}, argocdEdges(g, "deploys_to"), "edge-proxy's named destination has no cluster link")
	require.ElementsMatch(t, []string{
		"gcp.k8s.deployment:checkout -> argocd.application:shop-checkout",
		"gcp.k8s.deployment:catalog -> argocd.application:shop-catalog",
	}, argocdEdges(g, "deployed_by"))
	require.Equal(t, []string{"argocd.application:shop-catalog -> argocd.applicationset:shop"}, argocdEdges(g, "generated_by"))

	inCluster := map[string]bool{}
	for _, e := range argocdEdges(g, "in_cluster") {
		inCluster[e] = true
	}
	for _, src := range []string{
		"argocd.application:shop-catalog", "argocd.application:edge-proxy",
		"argocd.applicationset:shop", "argocd.applicationset:platform",
	} {
		require.True(t, inCluster[src+" -> "+batch], "%s lives in the cluster Argo CD runs in", src)
	}

	// The inspector's Argo CD section on the workload an ApplicationSet's
	// Application deploys.
	found = 0
	for _, r := range g.Resources {
		if r.Type != "gcp.k8s.deployment" || r.Name != "catalog" {
			continue
		}
		found++
		rows := map[string]any{}
		for _, d := range r.Details {
			if d.Section == "Argo CD" {
				rows[d.Key] = d.Value
			}
		}
		require.Equal(t, map[string]any{
			"application": "shop-catalog", "sync status": "OutOfSync", "health": "Healthy",
			"last deploy": "2026-11-14T16:30:00Z", "repo": "https://git.example.com/example-org/example-deploy.git",
			"path": "shop/catalog", "target revision": "main", "synced revision": "89abcdef0123456789abcdef0123456789abcdef",
		}, rows)
	}
	require.Equal(t, 1, found)
}

// TestArgoCDPackDeployedByChecks checks the deployed_by join against what
// the workloads say: checkout's instance label, and catalog's
// app.kubernetes.io/instance label and tracking-id annotation, the way
// Argo CD tracks resources by annotation.
func TestArgoCDPackDeployedByChecks(t *testing.T) {
	argocdWorkspace(t, argocdGraphModels...)
	apps := appNames(t)
	workloads := deploymentNames(t)

	var confirmed []string
	for _, args := range queryArgs(t, "argocd_workload_app_confirmed") {
		confirmed = append(confirmed, workloads[args["workload"].(string)]+" by "+
			apps[args["application"].(string)]+" via "+args["via"].(string))
	}
	require.ElementsMatch(t, []string{
		"checkout by shop-checkout via argocd.argoproj.io/instance",
		"catalog by shop-catalog via app.kubernetes.io/instance",
		"catalog by shop-catalog via argocd.argoproj.io/tracking-id",
	}, confirmed)

	endpoints := map[string]bool{}
	for _, args := range queryArgs(t, "gcp_gke_cluster_endpoint") {
		endpoints[args["host"].(string)] = true
	}
	require.Equal(t, map[string]bool{"198.51.100.30": true, argocdFixtureCluster: true}, endpoints,
		"public and DNS endpoints; batch-private's private endpoint is not one")
}
