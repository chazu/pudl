package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gcpDeployTypes are the build and deploy collectors.
var gcpDeployTypes = map[string]string{
	"build-triggers":      "gcp.#BuildTrigger",
	"appengine-apps":      "gcp.#AppEngineApplication",
	"appengine-services":  "gcp.#AppEngineService",
	"appengine-versions":  "gcp.#AppEngineVersion",
	"functions":           "gcp.#CloudFunction",
	"run-domain-mappings": "gcp.#RunDomainMapping",
}

// The build and deploy fixtures: App Engine's legacy duplicates are dropped,
// environment and substitution values never reach storage while their names
// do, and the facts read the shaped records.
func TestGCPPackDeployTypes(t *testing.T) {
	pudlDir := cliWorkspace(t)
	require.NoError(t, runCLI(t, "pack", "install", "gcp").Err)

	args := []string{"check"}
	for typ := range gcpDeployTypes {
		args = append(args, "gcp-fixture-"+typ)
	}
	r := runCLI(t, append(args, "--json")...)
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	counts := map[string]int{
		"gcp.#BuildTrigger": 3, "gcp.#AppEngineApplication": 1, "gcp.#AppEngineService": 2,
		"gcp.#AppEngineVersion": 2, "gcp.#CloudFunction": 2, "gcp.#RunDomainMapping": 1,
	}
	records := map[string][]map[string]any{}
	for _, schema := range gcpDeployTypes {
		records[schema] = listedRecords(t, schema)
		require.Len(t, records[schema], counts[schema], schema)
		for _, rec := range records[schema] {
			require.Equal(t, "example-project", rec["project"], schema)
		}
	}

	// Nothing marked as a fixture secret is stored, anywhere.
	require.NoError(t, filepath.WalkDir(filepath.Join(pudlDir, "data"), func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, secret := range []string{"not-a-real", "fixture-creator@", "fixture-modifier@"} {
			require.NotContains(t, string(raw), secret, path)
		}
		return nil
	}))

	byName := func(schema, key, name string) map[string]any {
		for _, rec := range records[schema] {
			if v, _ := rec[key].(string); strings.HasSuffix(v, name) {
				return rec
			}
		}
		t.Fatalf("%s %s not stored", schema, name)
		return nil
	}
	trigger := byName("gcp.#BuildTrigger", "name", "deploy-orders-api")
	require.Equal(t, "[REDACTED]", trigger["substitutions"])
	require.Equal(t, []any{"_APP_NAME", "_NOTIFY_WEBHOOK"}, trigger["substitutionNames"])
	require.Equal(t, "global", trigger["location"])
	require.Equal(t, "us-central1", byName("gcp.#BuildTrigger", "name", "nightly-export")["location"])

	version := byName("gcp.#AppEngineVersion", "name", "/versions/v1")
	require.Equal(t, "[REDACTED]", version["envVariables"])
	require.Equal(t, []any{"DATABASE_URL", "LOG_LEVEL"}, version["envVariableNames"])

	fn := byName("gcp.#CloudFunction", "name", "/order-events")
	require.Equal(t, "us-central1", fn["location"])
	service := fn["serviceConfig"].(map[string]any)
	require.Equal(t, "[REDACTED]", service["environmentVariables"])
	require.Equal(t, []any{"ORDERS_TOPIC", "WEBHOOK_SECRET"}, service["environmentVariableNames"])
	secrets, err := json.Marshal(service["secretEnvironmentVariables"])
	require.NoError(t, err)
	require.Contains(t, string(secrets), "orders-db-password", "secret references are kept")

	r = runCLI(t, "query", "gcp_build_trigger", "name=deploy-orders-api")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, `"service_account":"deployer@example-project.iam.gserviceaccount.com"`)
	r = runCLI(t, "query", "gcp_appengine_version", "service=apps/example-project/services/api")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, `"serving_status":"STOPPED"`)
	// Only a function with a Cloud Run service names one.
	r = runCLI(t, "query", "gcp_function_run_service")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, 1, strings.Count(r.Stdout, `"service":`), r.Stdout)
	require.Contains(t, r.Stdout, `"service":"projects/example-project/locations/us-central1/services/order-events"`)
	r = runCLI(t, "query", "gcp_run_domain_mapping")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, `"domain":"orders.example.com"`)
	require.Contains(t, r.Stdout, `"route":"orders-api"`)

	// gcloud lists triggers per location (the fixture's are global): its
	// records equal CAI's, and its observation is incomplete, so the
	// regional trigger it did not look for is kept.
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "gcp_settings.cue"), "package models\n\n_gcpAssetScope: \"off\"\n")
	pr := checkFixtureCollectors(t, []string{"build-triggers"})["gcp-fixture-build-triggers"]
	require.Equal(t, 2, pr.Records)
	require.Zero(t, pr.NewRecords, "gcloud's triggers are identical to CAI's")
	require.Len(t, listedRecords(t, "gcp.#BuildTrigger"), 3)
}
