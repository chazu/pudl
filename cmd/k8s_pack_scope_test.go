package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// k8sLiveCollectors are the pack's live collectors, one scope per cluster
// host each.
var k8sLiveCollectors = map[string]string{
	"k8s-cluster": "cluster", "k8s-deployments": "deployments", "k8s-statefulsets": "statefulsets",
	"k8s-daemonsets": "daemonsets", "k8s-services": "services", "k8s-ingresses": "ingresses",
	"k8s-namespaces": "namespaces", "k8s-nodes": "nodes",
}

// k8sObservation is one input's observation in a run report.
type k8sObservation struct {
	Input      string `json:"input"`
	Scope      string `json:"scope"`
	SnapshotID string `json:"snapshot_id"`
	Complete   bool   `json:"complete"`
}

// k8sMember is one model's report in `pudl check --json`.
type k8sMember struct {
	Model  string `json:"model"`
	Report struct {
		Populate *struct {
			SnapshotID   string           `json:"snapshot_id"`
			Observations []k8sObservation `json:"observations"`
		} `json:"populate"`
		Checks []struct {
			Name     string `json:"name"`
			Outcome  string `json:"outcome"`
			Evidence []struct {
				Scope    string `json:"scope"`
				Complete bool   `json:"complete"`
			} `json:"evidence"`
		} `json:"checks"`
	} `json:"report"`
}

// checkK8s runs `pudl check MODELS --json` and returns its members by model.
func checkK8s(t *testing.T, models ...string) map[string]k8sMember {
	t.Helper()
	r := runCLI(t, append(append([]string{"check"}, models...), "--json")...)
	var report struct {
		Members []k8sMember `json:"members"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout+r.Stderr)
	out := map[string]k8sMember{}
	for _, m := range report.Members {
		out[m.Model] = m
	}
	require.Len(t, out, len(models), r.Stdout)
	return out
}

// observeLive runs every live collector and requires each to observe host
// in one complete scope of its own, k8s/<host>/<kind>.
func observeLive(t *testing.T, host string) map[string]k8sObservation {
	t.Helper()
	models := make([]string, 0, len(k8sLiveCollectors))
	for model := range k8sLiveCollectors {
		models = append(models, model)
	}
	out := map[string]k8sObservation{}
	for model, m := range checkK8s(t, models...) {
		require.NotNil(t, m.Report.Populate, model)
		require.Empty(t, m.Report.Populate.SnapshotID, model)
		require.Len(t, m.Report.Populate.Observations, 1, model)
		o := m.Report.Populate.Observations[0]
		require.Equal(t, k8sObservation{Input: host, Scope: "k8s/" + host + "/" + k8sLiveCollectors[model], SnapshotID: o.SnapshotID, Complete: true}, o, model)
		out[model] = o
	}
	return out
}

// deploymentsPerCluster counts the k8s_deployment facts of each cluster.
func deploymentsPerCluster(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, args := range queryArgs(t, "k8s_deployment") {
		out[args["cluster"].(string)]++
	}
	return out
}

// TestK8sPackLivePostureChecks runs the live workload collectors and the
// live posture family: the collectors are complete, so the family reaches
// a verdict over the observed cluster, the fixtures without the hostile
// ones.
func TestK8sPackLivePostureChecks(t *testing.T) {
	useFakeKubectl(t)
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	checks := checkPack(t, "k8s-", k8sWorkloadTypes, []string{"posture"}, 2)["posture"]
	require.Len(t, checks, 4)
	privileged := requireCheck(t, checks, "privileged-container", "fail", 1)
	require.Equal(t, []string{"DaemonSet logging/log-agent agent"}, postureFindings(privileged))
	requireCheck(t, checks, "unpinned-image", "fail", 3)
	requireCheck(t, checks, "no-resource-limits", "fail", 5)
	requireCheck(t, checks, "may-run-as-root", "fail", 4)
}

// TestK8sPackLiveContextSwitch observes cluster A, switches the context to
// cluster B and observes again: A's records stay current and the posture
// family covers B only. Re-observing A without one deployment marks that
// deployment removed.
func TestK8sPackLiveContextSwitch(t *testing.T) {
	useFakeKubectl(t)
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	a, b := k8sFixtureHost, "203.0.113.7"
	first := observeLive(t, a)
	t.Setenv("K8S_SERVER", "https://"+b)
	observeLive(t, b)
	require.Equal(t, map[string]int{a: 3, b: 3}, deploymentsPerCluster(t), "observing B leaves A's deployments current")
	require.Len(t, queryArgs(t, "k8s_cluster"), 2)

	posture := checkK8s(t, "k8s-posture-checks")["k8s-posture-checks"]
	require.Len(t, posture.Report.Checks, 4)
	for _, c := range posture.Report.Checks {
		require.Equal(t, "fail", c.Outcome, c.Name)
		var scopes []string
		for _, e := range c.Evidence {
			require.True(t, e.Complete, c.Name)
			scopes = append(scopes, e.Scope)
		}
		require.ElementsMatch(t, []string{"k8s/" + b + "/deployments", "k8s/" + b + "/statefulsets", "k8s/" + b + "/daemonsets"}, scopes, c.Name)
	}

	// Back on A, without the catalog deployment.
	fixtures := withoutDeployment(t, "catalog")
	t.Setenv("K8S_SERVER", "https://"+a)
	t.Setenv("K8S_FIXTURES", fixtures)
	again := checkK8s(t, "k8s-deployments")["k8s-deployments"].Report.Populate.Observations
	require.Len(t, again, 1)
	require.Equal(t, "k8s/"+a+"/deployments", again[0].Scope)
	r = runCLI(t, "snapshot", "show", again[0].SnapshotID, "--compare", first["k8s-deployments"].SnapshotID, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var cmp struct {
		Changes []struct {
			Resource string `json:"resource"`
			Change   string `json:"change"`
		} `json:"changes"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &cmp), r.Stdout)
	require.Len(t, cmp.Changes, 1, r.Stdout)
	require.Equal(t, "removed", cmp.Changes[0].Change)
	require.Contains(t, cmp.Changes[0].Resource, "catalog")
}

// withoutDeployment copies the fake kubectl's fixtures (K8S_FIXTURES) to a
// new directory with the named deployment left out of deployments.json, and
// returns it.
func withoutDeployment(t *testing.T, name string) string {
	t.Helper()
	src := os.Getenv("K8S_FIXTURES")
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		require.NoError(t, err)
		if e.Name() == "deployments.json" {
			var list map[string]any
			require.NoError(t, json.Unmarshal(b, &list))
			var kept []any
			for _, item := range list["items"].([]any) {
				if item.(map[string]any)["metadata"].(map[string]any)["name"] != name {
					kept = append(kept, item)
				}
			}
			require.Len(t, kept, len(list["items"].([]any))-1)
			list["items"] = kept
			b, err = json.Marshal(list)
			require.NoError(t, err)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644))
	}
	return dst
}
