package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	pulumiDevSecret    = "urn:pulumi:dev::example-project::example:index:App$kubernetes:core/v1:Secret::app-db"
	pulumiDevConfigMap = "urn:pulumi:dev::example-project::example:index:App$kubernetes:core/v1:ConfigMap::app-config"
	pulumiDevSQLUser   = "urn:pulumi:dev::example-project::gcp:sql/user:User::app"
	pulumiProdSubnet   = "urn:pulumi:prod::example-network::example:index:Network$example:index:Subnets$gcp:compute/subnetwork:Subnetwork::core-a"
)

// TestPulumiPackFixtureResources runs the fixture resource collector over a
// hostile state: only the allow-listed fields are kept, no planted value
// reaches any stored byte, and the facts feed pulumi_managed and
// pulumi_ancestor.
func TestPulumiPackFixtureResources(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "pulumi", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "check", "pulumi-fixture-resources", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	report := checkedModel(t, r.Stdout)
	require.NotNil(t, report.Populate)
	require.Equal(t, 12, report.Populate.Records)

	byURN := map[string]map[string]any{}
	for _, rec := range storedRecords(t, "pulumi.#Resource") {
		byURN[rec["urn"].(string)] = rec
	}
	require.Len(t, byURN, 12)
	require.Equal(t, map[string]any{
		"urn": pulumiDevSecret, "stack": "example-org/example-project/dev",
		"type": "kubernetes:core/v1:Secret", "id": "apps/app-db", "custom": true, "external": false,
		"parent":       "urn:pulumi:dev::example-project::example:index:App::app",
		"provider":     "urn:pulumi:dev::example-project::pulumi:providers:kubernetes::cluster",
		"dependencies": []any{"urn:pulumi:dev::example-project::pulumi:providers:kubernetes::cluster"},
		"created":      "2026-01-01T00:00:00.000Z", "modified": "2026-01-01T00:00:00.000Z",
		"live": map[string]any{"metadata": map[string]any{
			"uid": "11111111-1111-4111-8111-111111111111", "name": "app-db", "namespace": "apps",
		}},
	}, byURN[pulumiDevSecret])
	require.Equal(t, map[string]any{"uid": "22222222-2222-4222-8222-222222222222", "namespace": "apps"},
		byURN[pulumiDevConfigMap]["live"].(map[string]any)["metadata"], "a secret envelope at an identity path is dropped")
	sqlUser := byURN[pulumiDevSQLUser]
	require.Equal(t, true, sqlUser["protect"])
	require.Equal(t, "project:///main.go#42", sqlUser["sourcePosition"])
	require.NotContains(t, sqlUser, "live", "only kubernetes types keep live values so far")

	requireNoPlanted(t, pudlDir, "PLANTED-", "planted-", "UExBTlRFRC1zZWNyZXQtZGF0YQ==")

	// Providers count as managed; components and external resources do not.
	requireQuery(t, 6, "pulumi_managed", `"urn":"`+pulumiProdSubnet+`"`)
	requireQuery(t, 0, "pulumi_managed", "", "urn=urn:pulumi:prod::example-network::gcp:compute/network:Network::shared")
	requireQuery(t, 3, "pulumi_ancestor", `"ancestor":"urn:pulumi:prod::example-network::pulumi:pulumi:Stack::example-network-prod"`, "urn="+pulumiProdSubnet)
	requireQuery(t, 0, "pulumi_ancestor", "", "urn=urn:pulumi:prod::example-network::pulumi:pulumi:Stack::example-network-prod")
}

// TestPulumiPackStackSourceFacts projects each stack's source.
func TestPulumiPackStackSourceFacts(t *testing.T) {
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "pulumi", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "check", "pulumi-fixture-stacks", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	requireQuery(t, 1, "pulumi_stack_source", `"dirty":true`, "stack=example-org/example-network/prod")
	requireQuery(t, 1, "pulumi_stack_source", `"root":"stacks/app"`, "stack=example-org/example-project/dev", "dirty=false")
}

// TestPulumiPackLiveResources runs pulumi-resources against a fake Pulumi
// Cloud: the guard, both stack list pages, and an export with no resources
// array, which is a stack with no records, not a failed input.
func TestPulumiPackLiveResources(t *testing.T) {
	pudlDir, api := pulumiLiveWorkspace(t)
	res, err := pulumiLiveRun(t, pudlDir, api, "pulumi-resources")()
	require.NoError(t, err)
	require.Len(t, res.Inputs, 3)
	require.Empty(t, res.Failed)
	require.Len(t, res.Records, 12)
	for _, rec := range res.Records {
		obj := rec.(map[string]any)
		require.NotContains(t, obj, "inputs")
		require.NotContains(t, obj, "outputs")
	}
	require.Contains(t, api.seen(), "GET /api/stacks/example-org/example-network/scratch/export token tok-pulumi-test")
}
