package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphexport"
)

// k8sGraphModels collect the GKE clusters and their deployments (GCP pack)
// and every Kubernetes pack fixture: the cluster, workloads and objects.
func k8sGraphModels() []string {
	models := []string{"gcp-fixture-gke-clusters", "gcp-fixture-k8s-deployments", "k8s-fixture-cluster"}
	models = append(models, k8sWorkloadFixtures...)
	return append(models, k8sObjectFixtures...)
}

// k8sNodeSchemas lists the schemas of a graph node's records.
func k8sNodeSchemas(r graphexport.Resource) []string {
	if len(r.Representations) == 0 {
		return []string{r.Schema}
	}
	var out []string
	for _, rep := range r.Representations {
		out = append(out, rep.Schema)
	}
	return out
}

// isK8sObject reports whether a graph node is a Kubernetes object: a
// Kubernetes pack record other than the cluster, or a GCP record of an
// object in a GKE cluster.
func isK8sObject(r graphexport.Resource) bool {
	for _, s := range k8sNodeSchemas(r) {
		if s == "gcp.#K8sDeployment" || (strings.HasPrefix(s, "k8s.#") && s != "k8s.#Cluster") {
			return true
		}
	}
	return false
}

// TestK8sPackGraph installs the GCP and Kubernetes packs, collects their
// fixtures and checks the exported graph: the GKE cluster prod-private and
// the kube context's cluster are one node, the GCP and kubectl views of a
// deployment are one node, every object has one in_cluster edge to the
// merged cluster, an Ingress routes to the Services its paths name, and
// every edge ends at a resource.
func TestK8sPackGraph(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, append(append([]string{"check"}, k8sGraphModels()...), "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	g := exportGraph(t)
	require.Empty(t, g.Meta.Warnings)
	requireCompleteVocabulary(t, g)

	// (1) prod-private is one node for its GKE record and the kube
	// context's cluster record.
	var cluster graphexport.Resource
	for _, res := range g.Resources {
		if res.Type == "gcp.container.cluster" && res.Name == "prod-private" {
			require.Empty(t, cluster.ID, "prod-private is one node")
			cluster = res
		}
		require.NotEqual(t, []string{"k8s.#Cluster"}, k8sNodeSchemas(res), "the cluster record has no node of its own")
	}
	require.ElementsMatch(t, []string{"gcp.#GKECluster", "k8s.#Cluster"}, k8sNodeSchemas(cluster))

	// (2) The GCP and kubectl views of each deployment GCP sees are one node.
	views := map[string][]string{}
	for _, res := range g.Resources {
		if res.Type == "gcp.k8s.deployment" || res.Type == "k8s.deployment" {
			views[res.Name] = append(views[res.Name], k8sNodeSchemas(res)...)
		}
	}
	for _, name := range []string{"checkout", "catalog"} {
		require.ElementsMatch(t, []string{"gcp.#K8sDeployment", "k8s.#Deployment"}, views[name], "shop/%s", name)
	}

	// (3) Every Kubernetes object has exactly one in_cluster edge, and it
	// ends at the merged cluster.
	inCluster := map[string][]string{}
	for _, rel := range g.Relationships {
		if rel.Type == "in_cluster" {
			inCluster[rel.Source] = append(inCluster[rel.Source], rel.Target)
		}
	}
	objects := map[string]int{}
	for _, res := range g.Resources {
		if !isK8sObject(res) {
			continue
		}
		objects[res.Type]++
		require.Equal(t, []string{cluster.ID}, inCluster[res.ID], "%s %s", res.Type, res.Name)
	}
	for _, typ := range []string{
		"gcp.k8s.deployment", "k8s.deployment", "k8s.statefulset", "k8s.daemonset",
		"k8s.service", "k8s.ingress", "k8s.namespace", "k8s.node",
	} {
		require.Positive(t, objects[typ], "the fixtures have a %s", typ)
	}

	// The storefront Ingress routes to the Services in its namespace that
	// its paths name: edge-proxy is in another namespace, and the metrics
	// Ingress's metrics-store has no Service record. The hostile fixtures
	// hold one more pair.
	require.ElementsMatch(t, []string{
		"k8s.ingress:storefront -> k8s.service:checkout",
		"k8s.ingress:storefront -> k8s.service:catalog",
		"k8s.ingress:hostile-ingress -> k8s.service:hostile-service",
	}, argocdEdges(g, "routes_to"))

	// (5) Every in_cluster and routes_to edge ends at a resource. Other GCP
	// edges can end at a record these fixtures do not collect (a KMS key).
	for _, rel := range g.Relationships {
		if rel.Type != "in_cluster" && rel.Type != "routes_to" {
			continue
		}
		require.False(t, strings.HasPrefix(rel.Source, graphexport.RefPrefix), rel.ID)
		require.False(t, strings.HasPrefix(rel.Target, graphexport.RefPrefix), rel.ID)
	}

	// The pack's rules pass doctor with the GCP pack's.
	r = runCLI(t, "doctor", "--json")
	var report doctorReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	var rules *healthDiagnostic
	for i, h := range report.Health {
		if h.Name == "Datalog Rules" {
			rules = &report.Health[i]
		}
	}
	require.NotNil(t, rules, r.Stdout)
	require.Equal(t, "ok", rules.Status, "%s\n%s", rules.Message, rules.Details)
}
