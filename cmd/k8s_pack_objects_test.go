package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// k8sObjectFixtures are the Kubernetes pack's Service, Ingress, Namespace
// and Node fixture collectors.
var k8sObjectFixtures = []string{"k8s-fixture-services", "k8s-fixture-ingresses", "k8s-fixture-namespaces", "k8s-fixture-nodes"}

// k8sObjectPlanted is every value the hostile object fixtures plant where
// the collectors must not keep it: the last-applied annotation of each
// kind, credential-named annotation values, URL userinfo in annotation
// values, managedFields, Node status beyond the kept fields, and objects in
// (or that are) system namespaces.
var k8sObjectPlanted = []string{
	"FAKE-K8S-LAST-APPLIED-SERVICE", "FAKE-K8S-LAST-APPLIED-INGRESS", "FAKE-K8S-LAST-APPLIED-NAMESPACE", "FAKE-K8S-LAST-APPLIED-NODE",
	"FAKE-K8S-SERVICE-TOKEN", "FAKE-K8S-SERVICE-USERINFO", "FAKE-K8S-INGRESS-USERINFO", "FAKE-K8S-AUTH-SECRET", "FAKE-K8S-AUTH-URL",
	"FAKE-K8S-MANAGED-FIELDS-SERVICE", "FAKE-K8S-MANAGED-FIELDS-INGRESS", "FAKE-K8S-MANAGED-FIELDS-NAMESPACE", "FAKE-K8S-MANAGED-FIELDS-NODE",
	"FAKE-K8S-NODE-IMAGE", "FAKE-K8S-NODE-VOLUME", "FAKE-K8S-NODE-STATUS-CONFIG",
	"FAKE-K8S-SYSTEM-SERVICE", "FAKE-K8S-SYSTEM-INGRESS", "FAKE-K8S-SYSTEM-NAMESPACE",
	// Any planted value at all.
	"FAKE-K8S",
}

// clusterScoped maps name to cluster for every tuple of a cluster-scoped
// relation.
func clusterScoped(t *testing.T, relation string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, args := range queryArgs(t, relation) {
		out[args["name"].(string)] = args["cluster"].(string)
	}
	return out
}

// tuples projects every tuple of relation onto the named args.
func tuples(t *testing.T, relation string, names ...string) [][]any {
	t.Helper()
	var out [][]any
	for _, args := range queryArgs(t, relation) {
		row := make([]any, len(names))
		for i, n := range names {
			row[i] = args[n]
		}
		out = append(out, row)
	}
	return out
}

// TestK8sPackFixtureObjects collects the Service, Ingress, Namespace and
// Node fixtures: a fact per object with its cluster, system namespaces
// (and the objects in them) skipped, every Node's provider_id, every
// Ingress host and path, and managedFields and Node status.images dropped.
func TestK8sPackFixtureObjects(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, append(append([]string{"check", "k8s-fixture-cluster"}, k8sObjectFixtures...), "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	h := k8sFixtureHost
	require.Equal(t, map[string]string{
		"default/kubernetes": h, "shop/checkout": h, "shop/catalog": h, "shop/cart-redis": h, "edge/edge-proxy": h,
		"hostile/hostile-service": h,
	}, workloadsByCluster(t, "k8s_service"), "kube-system and gke-* are skipped")
	require.ElementsMatch(t, [][]any{
		{"kubernetes", "ClusterIP", "10.8.0.1"}, {"checkout", "ClusterIP", "10.8.12.40"}, {"catalog", "ClusterIP", "10.8.12.41"},
		{"cart-redis", "ClusterIP", "None"}, {"edge-proxy", "LoadBalancer", "10.8.14.7"}, {"hostile-service", "ClusterIP", "10.8.20.1"},
	}, tuples(t, "k8s_service", "name", "type", "cluster_ip"))
	require.Len(t, queryArgs(t, "k8s_service_port"), 8, "one tuple per port")

	require.Equal(t, map[string]string{
		"shop/storefront": h, "monitoring/metrics": h, "hostile/hostile-ingress": h,
	}, workloadsByCluster(t, "k8s_ingress"), "gke-* and kube-system are skipped")
	require.ElementsMatch(t, [][]any{
		{"storefront", "shop.example.com"}, {"storefront", "api.example.com"}, {"hostile-ingress", "hostile.example.com"},
	}, tuples(t, "k8s_ingress_host", "name", "host"), "a rule without a host adds none")
	require.ElementsMatch(t, [][]any{
		{h, "shop", "storefront", "shop.example.com", "/", "checkout", 80.0},
		{h, "shop", "storefront", "shop.example.com", "/catalog", "catalog", "http"},
		{h, "shop", "storefront", "api.example.com", "/", "edge-proxy", 443.0},
		{h, "monitoring", "metrics", "", "/metrics", "metrics-store", 9090.0},
		{h, "hostile", "hostile-ingress", "hostile.example.com", "/", "hostile-service", 80.0},
	}, tuples(t, "k8s_ingress_backend", "cluster", "namespace", "name", "host", "path", "service", "port"))

	require.Equal(t, map[string]string{
		"default": h, "shop": h, "edge": h, "monitoring": h, "logging": h, "hostile": h,
	}, clusterScoped(t, "k8s_namespace"), "the system namespaces are skipped")

	require.ElementsMatch(t, [][]any{
		{h, "gke-prod-private-default-pool-1a2b3c4d-0001", "gce://example-project/us-central1-a/gke-prod-private-default-pool-1a2b3c4d-0001"},
		{h, "gke-prod-private-default-pool-1a2b3c4d-0002", "gce://example-project/us-central1-b/gke-prod-private-default-pool-1a2b3c4d-0002"},
		{h, "hostile-node", "gce://example-project/us-central1-c/hostile-node"},
	}, tuples(t, "k8s_node", "cluster", "name", "provider_id"))

	node := byName(storedRecords(t, "k8s.#Node"))["gke-prod-private-default-pool-1a2b3c4d-0001"]
	require.NotContains(t, node["metadata"], "managedFields")
	status := node["status"].(map[string]any)
	require.ElementsMatch(t, []string{"addresses", "nodeInfo", "conditions", "capacity", "allocatable"}, mapKeys(status),
		"images and daemonEndpoints are dropped")
	checkout := byName(storedRecords(t, "k8s.#Service"))["checkout"]
	require.Equal(t, `{"ingress":true}`, checkout["metadata"].(map[string]any)["annotations"].(map[string]any)["cloud.google.com/neg"])
}

// mapKeys returns the keys of m.
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestK8sPackHostileObjectsPlantNothing ingests the hostile object
// fixtures: the last-applied annotation and credential-named annotation
// values are redacted, URL userinfo is stripped, managedFields, Node
// status beyond the kept fields and system namespace objects are dropped.
// No planted byte is stored anywhere under the workspace.
func TestK8sPackHostileObjectsPlantNothing(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, append(append([]string{"check"}, k8sObjectFixtures...), "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	lastApplied := "kubectl.kubernetes.io/last-applied-configuration"
	annotations := func(schema, name string) map[string]any {
		records := byName(storedRecords(t, schema))
		require.NotContains(t, records, name+"-system", "%s: system namespaces are skipped", schema)
		rec := records[name]
		require.NotNil(t, rec, "%s: the hostile record is stored", schema)
		meta := rec["metadata"].(map[string]any)
		require.NotContains(t, meta, "managedFields", schema)
		return meta["annotations"].(map[string]any)
	}

	require.Equal(t, map[string]any{
		lastApplied:             "[REDACTED]",
		"example.com/api-token": "[REDACTED]",
		"example.com/upstream":  "redis://cache.hostile.svc:6379/0",
		"example.com/owner":     "platform",
	}, annotations("k8s.#Service", "hostile-service"))
	require.Equal(t, map[string]any{
		lastApplied: "[REDACTED]",
		"nginx.ingress.kubernetes.io/permanent-redirect": "https://auth.example.com/x",
		"nginx.ingress.kubernetes.io/auth-secret":        "[REDACTED]",
		"nginx.ingress.kubernetes.io/auth-url":           "[REDACTED]",
		"nginx.ingress.kubernetes.io/rewrite-target":     "/",
	}, annotations("k8s.#Ingress", "hostile-ingress"))
	require.Equal(t, map[string]any{lastApplied: "[REDACTED]", "example.com/owner": "platform"},
		annotations("k8s.#Namespace", "hostile"))
	require.NotContains(t, byName(storedRecords(t, "k8s.#Namespace")), "gke-hostile-system")
	nodeAnnotations := annotations("k8s.#Node", "hostile-node")
	require.Equal(t, "[REDACTED]", nodeAnnotations[lastApplied])
	require.Equal(t, "0", nodeAnnotations["node.alpha.kubernetes.io/ttl"], "other Node annotations are kept")
	status := byName(storedRecords(t, "k8s.#Node"))["hostile-node"]["status"].(map[string]any)
	require.ElementsMatch(t, []string{"addresses", "nodeInfo", "conditions", "capacity", "allocatable"}, mapKeys(status))

	requireNoPlanted(t, pudlDir, k8sObjectPlanted...)
}

// TestK8sPackLiveObjects runs the live collectors against a fake kubectl
// (fakeKubectl): Services and Ingresses are listed across all namespaces,
// Namespaces and Nodes without -A.
func TestK8sPackLiveObjects(t *testing.T) {
	fixtures, err := filepath.Abs("../packs/k8s/populators/k8s")
	require.NoError(t, err)
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "kubectl"), []byte(fakeKubectl), 0o755))
	log := filepath.Join(t.TempDir(), "kubectl.log")
	t.Setenv("KUBECTL_LOG", log)
	t.Setenv("K8S_FIXTURES", fixtures)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "check", "k8s-cluster", "k8s-services", "k8s-ingresses", "k8s-namespaces", "k8s-nodes", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	h := k8sFixtureHost
	require.Equal(t, map[string]string{
		"default/kubernetes": h, "shop/checkout": h, "shop/catalog": h, "shop/cart-redis": h, "edge/edge-proxy": h,
	}, workloadsByCluster(t, "k8s_service"))
	require.Equal(t, map[string]string{"shop/storefront": h, "monitoring/metrics": h}, workloadsByCluster(t, "k8s_ingress"))
	require.Equal(t, map[string]string{"default": h, "shop": h, "edge": h, "monitoring": h, "logging": h},
		clusterScoped(t, "k8s_namespace"))
	require.Equal(t, map[string]string{
		"gke-prod-private-default-pool-1a2b3c4d-0001": h, "gke-prod-private-default-pool-1a2b3c4d-0002": h,
	}, clusterScoped(t, "k8s_node"))

	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	for _, call := range []string{
		"get services -A -o json", "get ingresses.networking.k8s.io -A -o json", "get namespaces -o json", "get nodes -o json",
	} {
		require.Contains(t, string(calls), call+"\n")
	}
}
