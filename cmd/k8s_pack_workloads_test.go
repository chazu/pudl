package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/doctor"
)

// k8sWorkloadFixtures are the Kubernetes pack's workload fixture collectors.
var k8sWorkloadFixtures = []string{"k8s-fixture-deployments", "k8s-fixture-statefulsets", "k8s-fixture-daemonsets"}

// k8sPlanted is every value the hostile workload fixtures plant where the
// collectors must not keep it: env values, init container env values,
// container and init container command and args, the last-applied
// annotation, pod template annotation values, managedFields and objects in
// a system namespace.
var k8sPlanted = []string{
	"FAKE-K8S-ENV-VALUE", "FAKE-K8S-ENV-TOKEN", "FAKE-K8S-INIT-ENV-VALUE", "FAKE-K8S-LAST-APPLIED",
	"FAKE-K8S-TEMPLATE-ANNOTATION", "FAKE-K8S-TEMPLATE-CHECKSUM", "FAKE-K8S-MANAGED-FIELDS", "FAKE-K8S-SYSTEM",
	"FAKE-K8S-ARGS-DEPLOYMENT", "FAKE-K8S-COMMAND-DEPLOYMENT", "FAKE-K8S-ARGS-STATEFULSET", "FAKE-K8S-COMMAND-STATEFULSET",
	"FAKE-K8S-ARGS-DAEMONSET", "FAKE-K8S-COMMAND-DAEMONSET",
	// Any planted value at all.
	"FAKE-K8S",
}

// workloadsByCluster maps "<namespace>/<name>" to cluster for every tuple
// of a workload relation.
func workloadsByCluster(t *testing.T, relation string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, args := range queryArgs(t, relation) {
		out[args["namespace"].(string)+"/"+args["name"].(string)] = args["cluster"].(string)
	}
	return out
}

// TestK8sPackFixtureWorkloads collects the workload fixtures with the GCP
// fixtures: one record per object outside the system namespaces, without
// managedFields, a fact per object with its cluster, and the deployments
// GCP also sees linked to their gcp.#K8sDeployment by UID.
func TestK8sPackFixtureWorkloads(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	models := append([]string{"check", "gcp-fixture-gke-clusters", "gcp-fixture-k8s-deployments", "k8s-fixture-cluster"}, k8sWorkloadFixtures...)
	r = runCLI(t, append(models, "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	h := k8sFixtureHost
	require.Equal(t, map[string]string{
		"shop/checkout": h, "shop/catalog": h, "edge/edge-proxy": h, "hostile/hostile-deployment": h,
	}, workloadsByCluster(t, "k8s_deployment"), "kube-system is skipped")
	require.Equal(t, map[string]string{
		"shop/cart-redis": h, "monitoring/metrics-store": h, "hostile/hostile-statefulset": h,
	}, workloadsByCluster(t, "k8s_statefulset"), "gmp-system is skipped")
	require.Equal(t, map[string]string{
		"logging/log-agent": h, "hostile/hostile-daemonset": h,
	}, workloadsByCluster(t, "k8s_daemonset"), "kube-system and gke-* are skipped")

	replicas := map[string]any{}
	for _, args := range queryArgs(t, "k8s_statefulset") {
		replicas[args["name"].(string)] = args["replicas"]
	}
	require.Equal(t, 3.0, replicas["cart-redis"])

	deployments := byName(storedRecords(t, "k8s.#Deployment"))
	checkout := deployments["checkout"]
	require.Equal(t, h, checkout["cluster"])
	require.NotContains(t, checkout["metadata"], "managedFields")
	require.Equal(t, "7", checkout["metadata"].(map[string]any)["annotations"].(map[string]any)["deployment.kubernetes.io/revision"])
	env := checkout["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)[0].(map[string]any)["env"]
	require.Equal(t, []any{
		map[string]any{"name": "LOG_LEVEL", "value": "[REDACTED]"},
		map[string]any{"name": "PAYMENT_API_TOKEN", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "checkout-secrets", "key": "payment-api-token"}}},
	}, env, "env values are redacted; valueFrom references stay")
	template := byName(storedRecords(t, "k8s.#DaemonSet"))["log-agent"]["spec"].(map[string]any)["template"].(map[string]any)
	require.Equal(t, map[string]any{"prometheus.io/scrape": "[REDACTED]"}, template["metadata"].(map[string]any)["annotations"])

	// The two deployments the GCP fixture also holds share its UIDs.
	linked := map[string]string{}
	for _, e := range listEntries(t, "k8s.#Deployment") {
		var rec struct {
			Metadata struct{ Name string } `json:"metadata"`
		}
		require.NoError(t, json.Unmarshal(readStored(t, e.StoredPath), &rec))
		if g := showSameResource(t, e.ID); g != nil {
			require.Equal(t, []string{"k8s.object"}, g.Kinds)
			require.Len(t, g.Others, 1)
			linked[rec.Metadata.Name] = g.Others[0].Schema
		}
	}
	require.Equal(t, map[string]string{"checkout": "gcp.#K8sDeployment", "catalog": "gcp.#K8sDeployment"}, linked)
	check := doctor.CheckSameResource(workspaceSameResource(context.Background(), pudlDir))
	require.Equal(t, "ok", check.Status, "%s\n%s", check.Message, check.Details)
	require.Contains(t, check.Message, "3 group(s)", "the cluster and two deployments")
}

// TestK8sPackHostileWorkloadsPlantNothing ingests the hostile fixtures: env
// values, every command and args element, the last-applied annotation and
// pod template annotation values are redacted, managedFields and objects in system namespaces are
// dropped. No planted byte is stored anywhere under the workspace.
func TestK8sPackHostileWorkloadsPlantNothing(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, append(append([]string{"check"}, k8sWorkloadFixtures...), "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	for schema, name := range map[string]string{
		"k8s.#Deployment": "hostile-deployment", "k8s.#StatefulSet": "hostile-statefulset", "k8s.#DaemonSet": "hostile-daemonset",
	} {
		records := byName(storedRecords(t, schema))
		require.NotContains(t, records, name+"-system", "%s: system namespaces are skipped", schema)
		rec := records[name]
		require.NotNil(t, rec, "%s: the hostile record is stored", schema)
		meta := rec["metadata"].(map[string]any)
		require.NotContains(t, meta, "managedFields")
		require.Equal(t, map[string]any{
			"kubectl.kubernetes.io/last-applied-configuration": "[REDACTED]",
			"example.com/owner": "platform",
		}, meta["annotations"])
		template := rec["spec"].(map[string]any)["template"].(map[string]any)
		require.Equal(t, map[string]any{
			"vault.hashicorp.com/agent-inject-secret-db": "[REDACTED]", "checksum/config": "[REDACTED]",
		}, template["metadata"].(map[string]any)["annotations"], "%s: pod template annotation keys stay", schema)
		pod := template["spec"].(map[string]any)
		require.Equal(t, []any{
			map[string]any{"name": "DB_PASSWORD", "value": "[REDACTED]"},
			map[string]any{"name": "API_TOKEN", "value": "[REDACTED]"},
			map[string]any{"name": "DB_HOST", "valueFrom": map[string]any{"configMapKeyRef": map[string]any{"name": "hostile-config", "key": "db-host", "optional": true}}},
			map[string]any{"name": "DB_USER", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "hostile-db", "key": "user"}}},
		}, pod["containers"].([]any)[0].(map[string]any)["env"])
		require.Equal(t, []any{map[string]any{"name": "BOOTSTRAP_TOKEN", "value": "[REDACTED]"}},
			pod["initContainers"].([]any)[0].(map[string]any)["env"])
		redacted := []any{"[REDACTED]", "[REDACTED]"}
		for _, list := range []string{"containers", "initContainers"} {
			c := pod[list].([]any)[0].(map[string]any)
			require.Equal(t, redacted, c["command"], "%s: %s command elements are redacted", schema, list)
			require.Equal(t, redacted, c["args"], "%s: %s args elements are redacted", schema, list)
			require.Contains(t, c["image"], "registry.example.com/apps/hostile", "%s: the image stays", schema)
		}
	}

	requireNoPlanted(t, pudlDir, k8sPlanted...)
}

// fakeKubectl answers the read-only commands the live collectors run from
// the pack's fixtures: the cluster helper's config view and version, and
// `get <resource> [-A] -o json`. Every call is logged. K8S_SERVER switches
// the current context's API server (by default the fixture host's).
const fakeKubectl = `#!/bin/sh
echo "$*" >> "$KUBECTL_LOG"
server=${K8S_SERVER:-https://198.51.100.30}
case "$1 $2" in
"config view")
	case "$5" in
	'jsonpath={.contexts[0]}') echo '{"name":"live","context":{"cluster":"live","user":"live"}}' ;;
	'jsonpath={.clusters[0]}') echo '{"name":"live","cluster":{"server":"'"$server"'"}}' ;;
	'jsonpath={.clusters[0].cluster.server}') printf '%s' "$server" ;;
	esac ;;
"version -o") echo '{"serverVersion":{"gitVersion":"v1.30.5-gke.1014001"}}' ;;
"get deployments.apps") cat "$K8S_FIXTURES/deployments.json" ;;
"get statefulsets.apps") cat "$K8S_FIXTURES/statefulsets.json" ;;
"get daemonsets.apps") cat "$K8S_FIXTURES/daemonsets.json" ;;
"get services") cat "$K8S_FIXTURES/services.json" ;;
"get ingresses.networking.k8s.io") cat "$K8S_FIXTURES/ingresses.json" ;;
"get namespaces") cat "$K8S_FIXTURES/namespaces.json" ;;
"get nodes") cat "$K8S_FIXTURES/nodes.json" ;;
*) echo "unexpected kubectl $*" >&2; exit 1 ;;
esac
`

// useFakeKubectl puts fakeKubectl first on PATH for the test, answering
// from the pack's fixtures, and returns the path of its call log.
func useFakeKubectl(t *testing.T) string {
	t.Helper()
	fixtures, err := filepath.Abs("../packs/k8s/populators/k8s")
	require.NoError(t, err)
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "kubectl"), []byte(fakeKubectl), 0o755))
	log := filepath.Join(t.TempDir(), "kubectl.log")
	t.Setenv("KUBECTL_LOG", log)
	t.Setenv("K8S_FIXTURES", fixtures)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// TestK8sPackLiveWorkloads runs the live collectors against a fake kubectl:
// each lists its kind across all namespaces in the current context's
// cluster, and `pudl query` shows the StatefulSets and DaemonSets there.
func TestK8sPackLiveWorkloads(t *testing.T) {
	log := useFakeKubectl(t)
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "check", "k8s-cluster", "k8s-deployments", "k8s-statefulsets", "k8s-daemonsets", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	h := k8sFixtureHost
	require.Equal(t, map[string]string{"shop/checkout": h, "shop/catalog": h, "edge/edge-proxy": h},
		workloadsByCluster(t, "k8s_deployment"))
	require.Equal(t, map[string]string{"shop/cart-redis": h, "monitoring/metrics-store": h},
		workloadsByCluster(t, "k8s_statefulset"))
	require.Equal(t, map[string]string{"logging/log-agent": h}, workloadsByCluster(t, "k8s_daemonset"))
	require.Len(t, queryArgs(t, "k8s_cluster"), 1)

	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	for _, resource := range []string{"deployments.apps", "statefulsets.apps", "daemonsets.apps"} {
		require.Contains(t, string(calls), "get "+resource+" -A -o json\n")
	}
}
