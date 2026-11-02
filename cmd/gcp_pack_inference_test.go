package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// gcpPackSchemas maps each GCP pack fixture to the type its collector routes
// it to with an explicit schema.
var gcpPackSchemas = map[string]string{
	"networks": "gcp.#Network", "subnets": "gcp.#Subnetwork",
	"firewalls": "gcp.#Firewall", "routes": "gcp.#Route", "routers": "gcp.#Router",
	"addresses": "gcp.#Address", "gke-clusters": "gcp.#GKECluster",
	"sql-instances": "gcp.#SQLInstance", "buckets": "gcp.#Bucket",
	"run-services": "gcp.#RunService", "dns-zones": "gcp.#DNSManagedZone",
	"pubsub-topics": "gcp.#PubSubTopic", "pubsub-subscriptions": "gcp.#PubSubSubscription",
	"kms-keyrings": "gcp.#KMSKeyRing", "kms-keys": "gcp.#KMSKey",
}

// TestGCPPackFixturesInferTheirType imports each fixture without --schema,
// with the project injected as the collectors do, and requires inference to
// pick the pack type rather than a generic schema that shares field names
// (pudl/infra.#Service matches any record with `name` and `kind`).
func TestGCPPackFixturesInferTheirType(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "example", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Len(t, gcpPackSchemas, len(gcpPackTypes))

	for _, typ := range gcpPackTypes {
		schema, ok := gcpPackSchemas[typ]
		require.True(t, ok, typ)
		path := filepath.Join(pudlDir, "populators", "gcp", typ+".json")
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var records []map[string]any
		require.NoError(t, json.Unmarshal(raw, &records))

		r := runCLI(t, "import", "--path", path, "--set", "project=example-project")
		require.NoError(t, r.Err, r.Stdout+r.Stderr)

		r = runCLI(t, "list", "--json", "--all", "--items-only", "--schema", schema)
		require.NoError(t, r.Err, r.Stderr)
		var out struct {
			TotalMatched int `json:"total_matched"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
		require.Equal(t, len(records), out.TotalMatched, "%s records inferred as %s", typ, schema)
	}
}
