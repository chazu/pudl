package cmd

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/doctor"
)

// k8sFixtureHost is the API server host of the Kubernetes fixture cluster,
// the public endpoint of the GCP fixture GKE cluster prod-private.
const k8sFixtureHost = "198.51.100.30"

// TestK8sPackClusterSharesIdentityWithGKE installs the GCP and Kubernetes
// packs, collects both fixtures and checks that the kube context's cluster
// record and the GKE cluster with the same public endpoint are one
// k8s.cluster group, and that the other GKE clusters link to nothing.
func TestK8sPackClusterSharesIdentityWithGKE(t *testing.T) {
	pudlDir := cliWorkspace(t)
	require.NoError(t, runCLI(t, "pack", "install", "gcp").Err)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "check", "gcp-fixture-gke-clusters", "k8s-fixture-cluster", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	clusters := listEntries(t, "k8s.#Cluster")
	require.Len(t, clusters, 1)
	var cluster struct {
		Context, Cluster, Server, Host, ServerVersion string
	}
	require.NoError(t, json.Unmarshal(readStored(t, clusters[0].StoredPath), &cluster))
	require.Equal(t, k8sFixtureHost, cluster.Host)
	require.Equal(t, "https://"+k8sFixtureHost, cluster.Server)
	require.Equal(t, "gke_example-project_us-central1_prod-private", cluster.Context)
	require.Equal(t, cluster.Context, cluster.Cluster)
	require.Equal(t, "v1.30.5-gke.1014001", cluster.ServerVersion)

	shown := showSameResource(t, clusters[0].ID)
	require.NotNil(t, shown, "the cluster record must link to the GKE cluster")
	require.Equal(t, []string{"k8s.cluster"}, shown.Kinds)
	require.Len(t, shown.Others, 1)
	require.Equal(t, "gcp.#GKECluster", shown.Others[0].Schema)

	linked := map[string]bool{}
	for _, e := range listEntries(t, "gcp.#GKECluster") {
		var gke struct {
			Name string `json:"name"`
		}
		require.NoError(t, json.Unmarshal(readStored(t, e.StoredPath), &gke))
		linked[gke.Name] = showSameResource(t, e.ID) != nil
	}
	require.Equal(t, map[string]bool{"prod-private": true, "legacy-public": false, "batch-private": false}, linked)

	r = runCLI(t, "query", "k8s_cluster", "host="+k8sFixtureHost, "--json")
	require.NoError(t, r.Err, r.Stderr)
	var facts []struct {
		Args map[string]string `json:"args"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &facts), r.Stdout)
	require.Len(t, facts, 1)
	require.Equal(t, "v1.30.5-gke.1014001", facts[0].Args["server_version"])
	require.Equal(t, cluster.Context, facts[0].Args["context"])

	check := doctor.CheckSameResource(workspaceSameResource(context.Background(), pudlDir))
	require.Equal(t, "ok", check.Status, "%s\n%s", check.Message, check.Details)
	require.Contains(t, check.Message, "1 group(s)")
}

type listedEntry struct {
	ID         string `json:"id"`
	StoredPath string `json:"stored_path"`
}

// listEntries lists every current entry of a schema.
func listEntries(t *testing.T, schema string) []listedEntry {
	t.Helper()
	r := runCLI(t, "list", "--json", "--all", "--schema", schema)
	require.NoError(t, r.Err, r.Stderr)
	var listed struct {
		Entries []listedEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &listed), r.Stdout)
	return listed.Entries
}

func readStored(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}
