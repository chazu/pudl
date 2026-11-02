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

// TestGCPPackFixturesInferTheirType imports each fixture without --schema.
// Without a valid project ID every fixture is rejected; with the project
// injected as the collectors do, inference must pick the pack type rather than
// a generic schema that shares field names (the removed pudl/infra.#Service
// matched any record with `name` and `kind`).
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

		// Without a project ID the record claims its type and fails it, so the
		// import is rejected rather than stored under a generic schema. gcloud
		// prints the project for SQL instances, so only a bad --set fails there.
		invalid := [][]string{{"--set", "project="}, {"--set", "project=123456789012"}, {"--set", "project=My Project"}}
		if _, hasProject := records[0]["project"]; !hasProject {
			invalid = append(invalid, nil)
		}
		for _, set := range invalid {
			r := runCLI(t, append([]string{"import", "--path", path}, set...)...)
			require.Error(t, r.Err, "%s %v should be rejected", typ, set)
			require.Contains(t, r.Stderr, "@pudl(match)", "%s %v", typ, set)
		}

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
