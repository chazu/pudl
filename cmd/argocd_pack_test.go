package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// argocdFixtureCluster is the cluster the Argo CD fixtures were collected
// from: the DNS endpoint of the GCP fixture GKE cluster batch-private.
const argocdFixtureCluster = "gke-00000000000000000000000000000000-000000000000.us-central1.gke.goog"

// argocdPlanted is every value the hostile fixtures plant where the
// collectors must not keep it.
var argocdPlanted = []string{
	"FAKE-ARGO-LAST-APPLIED", "FAKE-ARGO-MANAGED-FIELDS", "FAKE-ARGO-HELM-VALUES",
	"FAKE-ARGO-VALUES-OBJECT", "FAKE-ARGO-HELM-PARAMETER", "FAKE-ARGO-FILE-PARAMETER",
	"FAKE-ARGO-PLUGIN-ENV", "FAKE-ARGO-PLUGIN-PARAMETER", "FAKE-ARGO-JSONNET-EXTVAR",
	"FAKE-ARGO-SPEC-INFO", "FAKE-ARGO-HISTORY-PARAMETER", "FAKE-ARGO-OPERATION-MESSAGE",
	"FAKE-ARGO-OPERATION-SOURCE", "FAKE-ARGO-SYNC-RESULT",
	"FAKE-ARGO-SET-LAST-APPLIED", "FAKE-ARGO-LIST-ELEMENT", "FAKE-ARGO-GENERATOR-TEMPLATE",
	"FAKE-ARGO-PLUGIN-INPUT", "FAKE-ARGO-CLUSTERS-VALUES", "FAKE-ARGO-TEMPLATE-PATCH",
	"FAKE-ARGO-TEMPLATE-VALUES", "FAKE-ARGO-TEMPLATE-VALUES-OBJECT", "FAKE-ARGO-TEMPLATE-PARAMETER",
	"FAKE-ARGO-TEMPLATE-FILE-PARAMETER",
	// Any planted value at all.
	"FAKE-ARGO",
}

// argocdWorkspace installs the Kubernetes, Argo CD and GCP packs in one
// command and runs the given fixture collectors.
func argocdWorkspace(t *testing.T, models ...string) string {
	t.Helper()
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s", "argocd", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "Installed argocd")
	r = runCLI(t, append(append([]string{"check"}, models...), "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	return pudlDir
}

// argsWhere keeps the tuples whose arg key holds value.
func argsWhere(tuples []map[string]any, key string, value any) []map[string]any {
	var out []map[string]any
	for _, args := range tuples {
		if args[key] == value {
			out = append(out, args)
		}
	}
	return out
}

// byName keys stored records by metadata.name.
func byName(records []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, rec := range records {
		out[rec["metadata"].(map[string]any)["name"].(string)] = rec
	}
	return out
}

// TestArgoCDPackFixtureApplications runs the Application fixture collector:
// one record per Application in the argocd namespace, shaped by the allow
// list, with spec.source and spec.sources as one sources list.
func TestArgoCDPackFixtureApplications(t *testing.T) {
	argocdWorkspace(t, "argocd-fixture-applications")

	apps := byName(storedRecords(t, "argocd.#Application"))
	require.Len(t, apps, 5, "four apps and the hostile one; sandbox-demo is outside _argocdNamespaces")
	require.NotContains(t, apps, "sandbox-demo")

	checkout := apps["shop-checkout"]
	require.Equal(t, argocdFixtureCluster, checkout["cluster"])
	require.Equal(t, "198.51.100.30", checkout["destinationHost"])
	require.Equal(t, []any{
		map[string]any{
			"index": 0.0, "repoURL": "https://git.example.com/example-org/example-deploy.git", "targetRevision": "main",
			"ref": "values", "syncRevision": "0123456789abcdef0123456789abcdef01234567",
		},
		map[string]any{
			"index": 1.0, "repoURL": "https://charts.example.com/stable", "chart": "web-service", "targetRevision": "2.1.0",
			"syncRevision": "2.1.0",
			"helm": map[string]any{
				"releaseName": "checkout", "valueFiles": []any{"$values/shop/checkout/values.yaml"},
				"parameters": []any{map[string]any{"name": "image.tag", "value": "1.4.2", "sourceIndex": 1.0}},
			},
		},
	}, checkout["sources"], "a multi-source app; each source has its synced revision")

	catalog := apps["shop-catalog"]
	require.Equal(t, "198.51.100.30", catalog["destinationHost"], "scheme, :443 and the trailing slash removed")
	require.Len(t, catalog["sources"], 1, "spec.source becomes a one-element list")
	require.Equal(t, "89abcdef0123456789abcdef0123456789abcdef", catalog["sources"].([]any)[0].(map[string]any)["syncRevision"])

	addons := apps["cluster-addons"]
	require.NotContains(t, addons, "destinationHost", "the in-cluster server has no host of its own")
	require.NotContains(t, addons["metadata"], "managedFields")
	require.NotContains(t, addons, "revisionHistoryLimit")
	status := addons["status"].(map[string]any)
	require.NotContains(t, status["operationState"], "syncResult")
	require.NotContains(t, status["operationState"], "message")
	for _, h := range status["history"].([]any) {
		require.NotContains(t, h, "source", "history entries drop their sources")
	}
	helm := addons["sources"].([]any)[0].(map[string]any)["helm"].(map[string]any)
	require.Equal(t, "[REDACTED]", helm["values"])
	require.Equal(t, []any{
		map[string]any{"name": "image.tag", "value": "4.2.0", "sourceIndex": 0.0},
		map[string]any{"name": "replicaCount", "value": "[REDACTED]", "sourceIndex": 0.0},
	}, helm["parameters"], "only image parameters keep their value")

	proxy := apps["edge-proxy"]
	require.Equal(t, map[string]any{"name": "prod-private", "namespace": "edge"}, proxy["destination"])
	require.NotContains(t, proxy, "destinationHost", "a named destination has no host")
	require.NotContains(t, proxy, "ignoreDifferences")
	require.Equal(t, map[string]any{
		"images": []any{"us-docker.pkg.dev/example-project/apps/edge-proxy:2.0.1"}, "namePrefix": "edge-",
	}, proxy["sources"].([]any)[0].(map[string]any)["kustomize"], "kustomize patches are dropped")

	// Facts.
	params := map[string]any{}
	for _, args := range queryArgs(t, "argocd_app_helm_parameter") {
		params[args["name"].(string)+"="+args["value"].(string)] = args["index"]
	}
	require.Equal(t, map[string]any{
		"image.tag=4.2.0": 0.0, "replicaCount=[REDACTED]": 0.0, "image.tag=1.4.2": 1.0,
		"db.password=[REDACTED]": 0.0, "image.tag=9.9.9": 0.0,
	}, params)
	updater := queryArgs(t, "argocd_app_image_updater")
	require.Len(t, updater, 1)
	require.Equal(t, "proxy=us-docker.pkg.dev/example-project/apps/edge-proxy", updater[0]["image_list"])
	require.Equal(t, "argocd", updater[0]["write_back_method"])
	kustomize := queryArgs(t, "argocd_app_kustomize_image")
	require.Len(t, kustomize, 1)
	require.Equal(t, "us-docker.pkg.dev/example-project/apps/edge-proxy:2.0.1", kustomize[0]["image"])
	sources := argsWhere(queryArgs(t, "argocd_app_source"), "ref", "values")
	require.Len(t, sources, 1)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", sources[0]["sync_revision"])
	require.Len(t, queryArgs(t, "argocd_app_history"), 6)
	require.Len(t, argsWhere(queryArgs(t, "argocd_app_resource"), "kind", "Deployment"), 4)
}

// TestArgoCDPackFixtureApplicationSets runs the ApplicationSet fixture
// collector: the template through the Application allow list, and a
// summary of each generator.
func TestArgoCDPackFixtureApplicationSets(t *testing.T) {
	argocdWorkspace(t, "argocd-fixture-applicationsets")

	sets := byName(storedRecords(t, "argocd.#ApplicationSet"))
	require.Len(t, sets, 4)
	shop := sets["shop"]
	require.Equal(t, []any{map[string]any{"type": "git", "git": map[string]any{
		"repoURL": "https://git.example.com/example-org/example-deploy.git", "revision": "main",
		"directories": []any{map[string]any{"path": "shop/*"}, map[string]any{"path": "shop/legacy", "exclude": true}},
	}}}, shop["generators"])
	tmpl := shop["template"].(map[string]any)
	require.Equal(t, "shop-{{.path.basename}}", tmpl["metadata"].(map[string]any)["name"])
	spec := tmpl["spec"].(map[string]any)
	require.Equal(t, "{{.path.path}}", spec["sources"].([]any)[0].(map[string]any)["path"], "template strings are kept as text")
	require.NotContains(t, spec, "destinationHost")

	require.Equal(t, []any{map[string]any{"type": "matrix", "generators": []any{
		map[string]any{"type": "clusters", "clusters": map[string]any{"selector": map[string]any{"matchLabels": map[string]any{"env": "prod"}}}},
		map[string]any{"type": "list", "list": map[string]any{"elementCount": 2.0}},
	}}}, sets["platform"]["generators"], "list elements are counted, not kept")

	hostile := sets["hostile-set"]
	require.NotContains(t, hostile, "templatePatch")
	helm := hostile["template"].(map[string]any)["spec"].(map[string]any)["sources"].([]any)[0].(map[string]any)["helm"].(map[string]any)
	require.Equal(t, "[REDACTED]", helm["values"])
	require.Equal(t, "[REDACTED]", helm["valuesObject"])

	types := map[string]string{}
	for _, args := range queryArgs(t, "argocd_appset") {
		types[args["name"].(string)] = args["generator_types"].(string)
	}
	require.Equal(t, map[string]string{
		"shop": "git", "platform": "matrix(clusters,list)", "regional": "merge(git,clusters)", "hostile-set": "list,plugin,clusters",
	}, types)
	git := queryArgs(t, "argocd_appset_git_generator")
	require.Len(t, git, 1)
	require.Equal(t, "main", git[0]["revision"])
	child := queryArgs(t, "argocd_appset_child_git_generator")
	require.Len(t, child, 1, "a git generator inside a merge")
	require.Equal(t, "release", child[0]["revision"])
}

// TestArgoCDPackHostileFixturesPlantNothing ingests the hostile fixtures:
// Helm values, valuesObject, non-image parameters, file parameter paths and
// the last-applied annotation are redacted; plugin env and parameters,
// jsonnet variables, history sources, the sync result, generator template
// overrides, list elements and templatePatch are dropped. No planted byte
// is stored anywhere under the workspace.
func TestArgoCDPackHostileFixturesPlantNothing(t *testing.T) {
	pudlDir := argocdWorkspace(t, "argocd-fixture-applications", "argocd-fixture-applicationsets")

	apps := byName(storedRecords(t, "argocd.#Application"))
	hostile := apps["hostile-app"]
	require.Equal(t, "[REDACTED]", hostile["metadata"].(map[string]any)["annotations"].(map[string]any)["kubectl.kubernetes.io/last-applied-configuration"])
	sources := hostile["sources"].([]any)
	require.Len(t, sources, 3)
	require.Equal(t, map[string]any{"name": "example-plugin"}, sources[1].(map[string]any)["plugin"])
	require.Equal(t, map[string]any{}, sources[2].(map[string]any)["directory"], "jsonnet is dropped")
	helm := sources[0].(map[string]any)["helm"].(map[string]any)
	require.NotContains(t, helm, "passCredentials")
	require.Equal(t, []any{map[string]any{"name": "tls.key", "path": "[REDACTED]"}}, helm["fileParameters"])

	requireNoPlanted(t, pudlDir, argocdPlanted...)
}
