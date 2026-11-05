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
	"instances": "gcp.#ComputeInstance", "service-accounts": "gcp.#ServiceAccount",
	"iam-policies": "gcp.#IAMPolicy", "iam-roles": "gcp.#IAMRole",
	"service-account-keys": "gcp.#ServiceAccountKey",
	"container-images":     "gcp.#ContainerImage",
	"k8s-deployments":      "gcp.#K8sDeployment",
	"organizations":        "gcp.#Organization", "folders": "gcp.#Folder",
	"org-policies": "gcp.#OrgPolicy", "firewall-policies": "gcp.#FirewallPolicy",
	"binauthz-policies": "gcp.#BinaryAuthorizationPolicy", "services": "gcp.#EnabledService",
	"forwarding-rules":        "gcp.#ForwardingRule",
	"target-http-proxies":     "gcp.#TargetHTTPProxy",
	"target-https-proxies":    "gcp.#TargetHTTPSProxy",
	"url-maps":                "gcp.#URLMap",
	"backend-services":        "gcp.#BackendService",
	"healthchecks":            "gcp.#HealthCheck",
	"http-healthchecks":       "gcp.#HTTPHealthCheck",
	"network-endpoint-groups": "gcp.#NetworkEndpointGroup",
	"ssl-certificates":        "gcp.#SSLCertificate",
	"ssl-policies":            "gcp.#SSLPolicy",
	"security-policies":       "gcp.#SecurityPolicy",
	"dns-record-sets":         "gcp.#DNSRecordSet",
}

// gcpPackOrgLevel are the fixtures of types above projects: their records
// name no project, so no project setting rejects them.
var gcpPackOrgLevel = map[string]bool{"organizations": true, "folders": true, "org-policies": true, "firewall-policies": true}

// gcpPackReshaped are the fixtures whose gcloud output is not the stored shape:
// get-iam-policy names no resource, so its collector's transform and set
// supply it; the container image helper prints gcloud's version list per
// image, which the transform turns into the image record. Imported as
// printed, such a record claims no pack type. Kubernetes deployments are
// collected from CAI only and have no gcloud fixture.
var gcpPackReshaped = map[string]bool{"iam-policies": true, "container-images": true, "k8s-deployments": true}

// TestGCPPackFixturesInferTheirType imports each fixture without --schema.
// Without a valid project ID every fixture is rejected; with the project
// injected as the collectors do, inference must pick the pack type rather than
// a generic schema that shares field names (the removed pudl/infra.#Service
// matched any record with `name` and `kind`).
func TestGCPPackFixturesInferTheirType(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Len(t, gcpPackSchemas, len(gcpPackTypes))

	for _, typ := range gcpPackTypes {
		schema, ok := gcpPackSchemas[typ]
		require.True(t, ok, typ)
		if gcpPackReshaped[typ] {
			continue
		}
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
		if gcpPackOrgLevel[typ] {
			invalid = nil
		}
		for _, set := range invalid {
			r := runCLI(t, append([]string{"import", "--path", path}, set...)...)
			require.Error(t, r.Err, "%s %v should be rejected", typ, set)
			require.Contains(t, r.Stderr, "@pudl(match)", "%s %v", typ, set)
		}

		set := []string{"--set", "project=example-project"}
		if gcpPackOrgLevel[typ] {
			set = nil
		}
		r := runCLI(t, append([]string{"import", "--path", path}, set...)...)
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
