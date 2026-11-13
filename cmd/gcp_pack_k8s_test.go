package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// queryArgs runs `pudl query` and returns the args of every tuple.
func queryArgs(t *testing.T, relation string) []map[string]any {
	t.Helper()
	r := runCLI(t, "query", relation, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var tuples []struct {
		Args map[string]any `json:"args"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &tuples), r.Stdout)
	out := make([]map[string]any, 0, len(tuples))
	for _, tuple := range tuples {
		out = append(out, tuple.Args)
	}
	return out
}

// deploymentNames maps each deployment's resource_id to its name.
func deploymentNames(t *testing.T) map[string]string {
	t.Helper()
	names := map[string]string{}
	for _, args := range queryArgs(t, "gcp_k8s_deployment") {
		names[args["resource_id"].(string)] = args["name"].(string)
	}
	return names
}

// containerFacts keys the tuples of a container relation by
// "<deployment>/<container>".
func containerFacts(t *testing.T, names map[string]string, relation string) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, args := range queryArgs(t, relation) {
		name, _ := args["container"].(string)
		out[names[args["resource_id"].(string)]+"/"+name] = args
	}
	return out
}

// The CAI fixture's deployments project one fact per container and per init
// container (image reference split into registry, tag and digest), the
// Artifact Registry image each runs, and the Argo CD instance labels.
func TestGCPPackK8sDeploymentFixture(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "gcp-fixture-k8s-deployments", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, 2, checkedModel(t, r.Stdout).Populate.Records, "the kube-system deployment is skipped")

	names := deploymentNames(t)
	require.Len(t, names, 2)
	containers := containerFacts(t, names, "gcp_k8s_deployment_container")
	require.Len(t, containers, 4)
	require.Equal(t, map[string]any{
		"image": "us-docker.pkg.dev/example-project/apps/checkout:1.4.2", "registry": "us-docker.pkg.dev", "tag": "1.4.2", "digest": "",
	}, imageParts(containers["checkout/checkout"]), "a tagged Artifact Registry image")
	require.Equal(t, map[string]any{
		"image": "gcr.io/example-project/api:3f2a9c1@sha256:1111111111111111111111111111111111111111111111111111111111111111", "registry": "gcr.io", "tag": "3f2a9c1",
		"digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
	}, imageParts(containers["checkout/proxy"]), "a tag and a digest")
	require.Equal(t, map[string]any{
		"image": "redis:7.2", "registry": "docker.io", "tag": "7.2", "digest": "",
	}, imageParts(containers["catalog/cache"]), "a Docker Hub reference names no registry")
	require.Equal(t, "catalog", containers["catalog/catalog"]["container"])

	init := containerFacts(t, names, "gcp_k8s_deployment_init_container")
	require.Len(t, init, 1)
	require.Equal(t, map[string]any{
		"image": "us-docker.pkg.dev/example-project/apps/checkout-migrate@sha256:3333333333333333333333333333333333333333333333333333333333333333", "registry": "us-docker.pkg.dev", "tag": "",
		"digest": "sha256:3333333333333333333333333333333333333333333333333333333333333333",
	}, imageParts(init["checkout/migrate"]), "a digest and no tag")

	// Images as gcp.#ContainerImage names them: tag and digest dropped,
	// gcr.io mapped to Artifact Registry; the Docker Hub image yields none.
	images := containerFacts(t, names, "gcp_k8s_deployment_image")
	require.Len(t, images, 3)
	require.Equal(t, "us-docker.pkg.dev/example-project/apps/checkout", images["checkout/checkout"]["image"])
	require.Equal(t, "us-docker.pkg.dev/example-project/gcr.io/api", images["checkout/proxy"]["image"])
	require.Equal(t, "us-docker.pkg.dev/example-project/apps/catalog", images["catalog/catalog"]["image"])
	initImages := containerFacts(t, names, "gcp_k8s_deployment_init_image")
	require.Len(t, initImages, 1)
	require.Equal(t, "us-docker.pkg.dev/example-project/apps/checkout-migrate", initImages["checkout/migrate"]["image"])

	// The Argo CD Application, from either instance label.
	argo := map[string]string{}
	for _, args := range queryArgs(t, "gcp_k8s_deployment_argo_app") {
		argo[names[args["resource_id"].(string)]] = args["application"].(string) + " via " + args["via"].(string)
	}
	require.Equal(t, map[string]string{
		"checkout": "shop-checkout via argocd.argoproj.io/instance",
		"catalog":  "shop-catalog via app.kubernetes.io/instance",
	}, argo)
	require.Len(t, queryArgs(t, "gcp_k8s_deployment_argo_instance"), 1, "a deployment without the label yields no fact")
	require.Len(t, queryArgs(t, "gcp_k8s_deployment_app_instance"), 1)
}

// imageParts picks the image reference parts of a container fact.
func imageParts(args map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"image", "registry", "tag", "digest"} {
		out[k] = args[k]
	}
	return out
}
