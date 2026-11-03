package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// gcpAssetTypes are the pack types collected through Cloud Asset Inventory.
var gcpAssetTypes = []string{
	"networks", "subnets", "firewalls", "routes", "routers", "addresses",
	"gke-clusters", "run-services", "pubsub-topics", "service-accounts", "dns-zones",
	"kms-keyrings", "kms-keys",
}

type assetPopulate struct {
	Records    int `json:"records"`
	NewRecords int `json:"new_records"`
	Fallback   any `json:"fallback"`
}

func checkFixtureCollectors(t *testing.T, types []string) map[string]assetPopulate {
	t.Helper()
	args := []string{"check"}
	for _, typ := range types {
		args = append(args, "gcp-fixture-"+typ)
	}
	r := runCLI(t, append(args, "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var check struct {
		Members []struct {
			Model  string `json:"model"`
			Report struct {
				Populate assetPopulate `json:"populate"`
			} `json:"report"`
		} `json:"members"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &check), r.Stdout)
	out := map[string]assetPopulate{}
	for _, m := range check.Members {
		out[m.Model] = m.Report.Populate
	}
	return out
}

// Each CAI type's fixture collector reads the bundled CAI search result; with
// _gcpAssetScope: "off" it reads the bundled gcloud output instead. Both store
// the same records, so the second run stores nothing new.
func TestGCPPackAssetSourcesStoreIdenticalRecords(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "example", "install", "gcp")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	first := checkFixtureCollectors(t, gcpAssetTypes)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"), "package models\n\n_gcpAssetScope: \"off\"\n")
	second := checkFixtureCollectors(t, gcpAssetTypes)
	require.Len(t, second, len(gcpAssetTypes))
	for model, pr := range second {
		require.Nil(t, pr.Fallback, model)
		require.Positive(t, pr.Records, model)
		require.Equal(t, first[model].Records, pr.Records, "%s: the CAI fixture's other asset types are dropped", model)
		require.Equal(t, first[model].NewRecords, pr.Records, "%s: everything was new from CAI", model)
		require.Zero(t, pr.NewRecords, "%s: gcloud's records are identical to CAI's", model)
	}
}
