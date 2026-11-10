package cmd

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/populate"
	"github.com/chazu/pudl/internal/systemmodel"
)

// TestPulumiPackFixtureStacks runs the fixture stack collector: one record
// per stack with an update, holding only the allow-listed fields.
func TestPulumiPackFixtureStacks(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "pulumi", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	r = runCLI(t, "check", "pulumi-fixture-stacks", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	report := checkedModel(t, r.Stdout)
	require.NotNil(t, report.Populate)
	require.Equal(t, 2, report.Populate.Records, "scratch has no update, so no record")

	stacks := map[string]map[string]any{}
	for _, rec := range storedRecords(t, "pulumi.#Stack") {
		stacks[rec["stack"].(string)] = rec
	}
	require.Len(t, stacks, 2)
	dev := stacks["example-org/example-project/dev"]
	require.Equal(t, map[string]any{
		"stack": "example-org/example-project/dev", "version": 42.0, "kind": "update", "result": "succeeded",
		"startTime": 1767225500.0, "endTime": 1767225600.0, "resourceCount": 3.0, "pulumiVersion": "v3.200.0",
		"source": map[string]any{
			"repo": "example-owner/example-infra", "vcs": "github.com", "root": "stacks/app",
			"commit": "0123456789abcdef0123456789abcdef01234567", "branch": "refs/heads/main", "dirty": false,
		},
	}, dev)
	prod := stacks["example-org/example-network/prod"]
	require.Equal(t, true, prod["source"].(map[string]any)["dirty"])
	require.NotContains(t, prod, "resourceCount")

	// The update's message, config, deployment and other environment keys
	// are not kept anywhere pudl stores data.
	requireNoPlanted(t, pudlDir, "PLANTED-", "planted-")
}

// storedRecords returns the stored records of one schema.
func storedRecords(t *testing.T, schema string) []map[string]any {
	t.Helper()
	r := runCLI(t, "list", "--schema", schema, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var listed struct {
		Entries []struct {
			StoredPath string `json:"stored_path"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &listed), r.Stdout)
	var out []map[string]any
	for _, e := range listed.Entries {
		raw, err := os.ReadFile(e.StoredPath)
		require.NoError(t, err)
		var rec map[string]any
		require.NoError(t, json.Unmarshal(raw, &rec))
		out = append(out, rec)
	}
	return out
}

// requireNoPlanted searches every file under pudlDir, the bundled fixtures
// excepted, for each planted marker.
func requireNoPlanted(t *testing.T, pudlDir string, markers ...string) {
	t.Helper()
	fixtures := filepath.Join(pudlDir, "populators")
	require.NoError(t, filepath.WalkDir(pudlDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(path, fixtures) {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range markers {
			require.NotContains(t, string(raw), m, "planted value in %s", path)
		}
		return nil
	}))
}

// pulumiAPI serves the pack's fixtures as Pulumi Cloud responses and records
// every request.
type pulumiAPI struct {
	*httptest.Server
	mu       sync.Mutex
	routes   map[string]string
	requests []string
}

func newPulumiAPI(t *testing.T, fixtures string) *pulumiAPI {
	t.Helper()
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(fixtures, name))
		require.NoError(t, err)
		return string(raw)
	}
	updates := func(name string) string { return `{"updates": ` + read("updates-"+name+".json") + `}` }
	a := &pulumiAPI{routes: map[string]string{
		"/api/user": read("guard-user-org-token.json"),
		"/api/orgs/example-org/roles?uxPurpose=role&assignedToMe=true":              read("guard-roles-assigned.json"),
		"/api/orgs/example-org/roles?uxPurpose=policy":                              read("guard-roles-policies.json"),
		"/api/orgs/example-org/roles?uxPurpose=set":                                 read("guard-roles-sets.json"),
		"/api/user/stacks?organization=example-org":                                 read("stacks-page-1.json"),
		"/api/user/stacks?continuationToken=page-2&organization=example-org":        read("stacks-page-2.json"),
		"/api/stacks/example-org/example-project/dev/updates?pageSize=1&page=1":     updates("dev"),
		"/api/stacks/example-org/example-network/prod/updates?pageSize=1&page=1":    updates("prod"),
		"/api/stacks/example-org/example-network/scratch/updates?pageSize=1&page=1": updates("scratch"),
	}}
	a.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.requests = append(a.requests, r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Authorization"))
		body, ok := a.routes[r.URL.RequestURI()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(a.Close)
	return a
}

// set replaces one response and clears the recorded requests.
func (a *pulumiAPI) set(path, body string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.routes[path] = body
	a.requests = nil
}

func (a *pulumiAPI) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.requests...)
}

// TestPulumiPackGuard runs pulumi-stacks against a fake Pulumi Cloud. Only
// the configured org token with a read-only role passes the guard; any
// other token or role is refused after the four guard requests.
func TestPulumiPackGuard(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "pulumi", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	api := newPulumiAPI(t, filepath.Join(pudlDir, "populators", "pulumi"))
	schemaDir := filepath.Join(pudlDir, "schema")
	require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "models", "pulumi_settings.cue"), []byte(
		"package models\n\n_pulumiOrg: \"example-org\"\n_pulumiTokenName: \"pudl-reader\"\n_pulumiAPI: \""+api.URL+"\"\n"), 0o600))

	tmpl, err := systemmodel.ResolveTemplate([]string{schemaDir}, "pulumi-stacks")
	require.NoError(t, err)
	model, err := tmpl.Model()
	require.NoError(t, err)
	run := func() (populate.Result, error) {
		return populate.Run(context.Background(), model.Populate, populate.Request{
			HTTPClient: api.Client(),
			Getenv: func(name string) string {
				if name == "PUDL_PULUMI_TOKEN" {
					return "tok-pulumi-test"
				}
				return ""
			},
		})
	}
	guardRequests := []string{
		"GET /api/orgs/example-org/roles?uxPurpose=policy token tok-pulumi-test",
		"GET /api/orgs/example-org/roles?uxPurpose=role&assignedToMe=true token tok-pulumi-test",
		"GET /api/orgs/example-org/roles?uxPurpose=set token tok-pulumi-test",
		"GET /api/user token tok-pulumi-test",
	}

	res, err := run()
	require.NoError(t, err)
	require.Equal(t, []string{
		"example-org/example-project/dev", "example-org/example-network/prod", "example-org/example-network/scratch",
	}, res.Inputs, "both pages of the stack list")
	require.Len(t, res.Records, 2)
	require.Empty(t, res.Failed)

	const (
		userPath   = "/api/user"
		rolePath   = "/api/orgs/example-org/roles?uxPurpose=role&assignedToMe=true"
		policyPath = "/api/orgs/example-org/roles?uxPurpose=policy"
		setPath    = "/api/orgs/example-org/roles?uxPurpose=set"
	)
	fixture := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(pudlDir, "populators", "pulumi", name))
		require.NoError(t, err)
		return string(raw)
	}
	role := func(details string) string {
		return `{"roles": [{"name": "r", "uxPurpose": "role", "id": "00000000-0000-4000-8000-000000000001", "details": ` + details + `}]}`
	}

	// A role that reaches its scopes through a policy, under a condition,
	// passes when every scope it reaches is a read.
	api.set(policyPath, strings.Replace(fixture("guard-roles-policies.json"), "00000000-0000-4000-8000-000000000012", "00000000-0000-4000-8000-000000000011", 1))
	api.set(rolePath, role(`{"__type": "PermissionDescriptorGroup", "entries": [{"__type": "PermissionDescriptorCompose", "permissionDescriptors": ["00000000-0000-4000-8000-000000000002"]}]}`))
	res, err = run()
	require.NoError(t, err)
	require.Len(t, res.Records, 2)
	api.set(policyPath, fixture("guard-roles-policies.json"))
	api.set(rolePath, fixture("guard-roles-assigned.json"))

	refused := []struct {
		name, path, body, reason string
	}{
		{"personal token", userPath, fixture("guard-user-personal-token.json"), "a personal or team token is refused"},
		{"team token", userPath, fixture("guard-user-team-token.json"), "a personal or team token is refused"},
		{"another org", userPath, strings.Replace(fixture("guard-user-org-token.json"), `"organization": "example-org"`, `"organization": "other-org"`, 1), "a personal or team token is refused"},
		{"another token", userPath, strings.Replace(fixture("guard-user-org-token.json"), `"name": "pudl-reader",
    "organization"`, `"name": "ci-deployer",
    "organization"`, 1), "a personal or team token is refused"},
		{"no role", rolePath, `{"roles": []}`, "exactly one role"},
		{"two roles", rolePath, strings.Replace(fixture("guard-roles-assigned.json"), `"roles": [`, `"roles": [{"id": "x", "details": {"__type": "PermissionDescriptorAllow", "permissions": []}},`, 1), "exactly one role"},
		{"decrypt inline", rolePath, role(`{"__type": "PermissionDescriptorAllow", "permissions": ["stack:read", "stack:decrypt"]}`), "outside pudl's read allow list: stack:decrypt"},
		{"write inline", rolePath, role(`{"__type": "PermissionDescriptorAllow", "permissions": ["stack:write"]}`), "outside pudl's read allow list: stack:write"},
		{"decrypt behind a policy condition", rolePath, role(`{"__type": "PermissionDescriptorCompose", "permissionDescriptors": ["00000000-0000-4000-8000-000000000002"]}`), "stack:decrypt, stack:encrypt"},
		{"unknown descriptor", rolePath, role(`{"__type": "PermissionDescriptorDeny", "permissions": []}`), "a descriptor pudl does not know"},
		{"unknown ID", rolePath, role(`{"__type": "PermissionDescriptorCompose", "permissionDescriptors": ["00000000-0000-4000-8000-0000000000ff"]}`), "a descriptor pudl does not know"},
		{"cycle", setPath, strings.Replace(fixture("guard-roles-sets.json"), `"__type": "PermissionDescriptorAllow",
        "permissions": [
          "stack:read",
          "stack:export"
        ]`, `"__type": "PermissionDescriptorCompose",
        "permissionDescriptors": ["00000000-0000-4000-8000-000000000011"]`, 1), "a descriptor pudl does not know"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			original := api.routes[tc.path]
			api.set(tc.path, tc.body)
			defer api.set(tc.path, original)
			if tc.name == "cycle" {
				api.set(rolePath, role(`{"__type": "PermissionDescriptorCompose", "permissionDescriptors": ["00000000-0000-4000-8000-000000000011"]}`))
				defer api.set(rolePath, fixture("guard-roles-assigned.json"))
			}
			_, err := run()
			require.ErrorContains(t, err, tc.reason)
			require.Equal(t, guardRequests, api.seen(), "nothing is sent after the guard")
		})
	}
}
