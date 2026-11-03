package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gcpPackTypes are the collectors of the GCP content pack, in install order.
var gcpPackTypes = []string{
	"networks", "subnets", "firewalls", "routes", "routers", "addresses",
	"gke-clusters", "sql-instances", "buckets", "run-services", "dns-zones",
	"pubsub-topics", "pubsub-subscriptions", "kms-keyrings", "kms-keys", "instances",
	"service-accounts", "iam-policies",
}

var gcpPackFamilies = []string{"network-hygiene", "data-protection", "gke-posture"}

// packCheck is the part of a member report's check result the test reads.
type packCheck struct {
	Name      string `json:"name"`
	Outcome   string `json:"outcome"`
	Severity  string `json:"severity"`
	Count     int    `json:"count"`
	Passed    bool   `json:"passed"`
	Witnesses []struct {
		Arguments map[string]any `json:"arguments"`
		Owner     string         `json:"owner"`
	} `json:"witnesses"`
}

// checkGCPPack runs every fixture collector and family in one `pudl check`,
// requires the exit code, and returns each family's checks by name.
func checkGCPPack(t *testing.T, exitCode int) map[string]map[string]packCheck {
	t.Helper()
	return checkPack(t, "gcp-fixture-", gcpPackTypes, gcpPackFamilies, exitCode)
}

// checkPack runs a content pack's fixture collectors (prefix + type) and
// families (prefix + family + "-checks") in one `pudl check`, requires the
// exit code, and returns each family's checks by name. extra flags are
// appended to the command.
func checkPack(t *testing.T, prefix string, types, familyNames []string, exitCode int, extra ...string) map[string]map[string]packCheck {
	t.Helper()
	args := []string{"check"}
	for _, typ := range types {
		args = append(args, prefix+typ)
	}
	for _, family := range familyNames {
		args = append(args, prefix+family+"-checks")
	}
	args = append(args, extra...)
	r := runCLI(t, append(args, "--json", "--detailed-exitcode")...)
	require.Equal(t, exitCode, exitCodeFor(r.Err), r.Stdout+r.Stderr)
	var check struct {
		Members []struct {
			Model  string `json:"model"`
			Result string `json:"result"`
			Report struct {
				Populate *struct {
					Records int `json:"records"`
				} `json:"populate"`
				Checks []packCheck `json:"checks"`
			} `json:"report"`
		} `json:"members"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &check), r.Stdout)
	require.Len(t, check.Members, len(types)+len(familyNames))
	families := map[string]map[string]packCheck{}
	for _, m := range check.Members {
		family, ok := strings.CutSuffix(strings.TrimPrefix(m.Model, prefix), "-checks")
		if !ok {
			require.Equal(t, "succeeded", m.Result, "collector %s", m.Model)
			continue
		}
		families[family] = map[string]packCheck{}
		for _, c := range m.Report.Checks {
			families[family][c.Name] = c
		}
	}
	require.Len(t, families, len(familyNames))
	return families
}

// witnessArgs collects one argument of every witness of a check.
func witnessArgs(c packCheck, arg string) []string {
	var out []string
	for _, w := range c.Witnesses {
		v, _ := w.Arguments[arg].(string)
		out = append(out, v[strings.LastIndex(v, "/")+1:])
	}
	return out
}

// kindCounts counts the tuples of `pudl query` output by their kind argument.
func kindCounts(stdout string) map[string]int {
	counts := map[string]int{}
	for _, line := range strings.Split(stdout, "\n") {
		_, rest, ok := strings.Cut(line, `"kind":"`)
		if !ok {
			continue
		}
		kind, _, _ := strings.Cut(rest, `"`)
		counts[kind]++
	}
	return counts
}

func requireCheck(t *testing.T, checks map[string]packCheck, name, outcome string, count int) packCheck {
	t.Helper()
	c, ok := checks[name]
	require.True(t, ok, "check %s missing", name)
	require.Equal(t, outcome, c.Outcome, name)
	require.Equal(t, count, c.Count, name)
	return c
}

func TestGCPContentPack(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "example", "install", "gcp", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	r = runCLI(t, "example", "install", "gcp", "--json")
	require.NoError(t, r.Err, "identical install is idempotent")

	families := checkGCPPack(t, 2)

	nh := families["network-hygiene"]
	high := requireCheck(t, nh, "policy-high", "fail", 1)
	require.Equal(t, "gcp.#FirewallBestPractice", high.Witnesses[0].Arguments["policy"])
	anyViolation := requireCheck(t, nh, "policy-any", "fail", 3) // the firewall plus two unused external addresses (low)
	require.Equal(t, "warn", anyViolation.Severity, "policy-any only warns")
	nat := requireCheck(t, nh, "nat-ip-not-in-use", "fail", 1)
	require.Equal(t, []string{"nat-ip-2"}, witnessArgs(nat, "address"))

	dp := families["data-protection"]
	requireCheck(t, dp, "policy-high", "fail", 4) // two buckets, two SQL instances
	requireCheck(t, dp, "policy-any", "fail", 10) // one tuple per resource and severity

	gke := families["gke-posture"]
	requireCheck(t, gke, "policy-high", "fail", 1)
	requireCheck(t, gke, "policy-any", "fail", 2)
	pga := requireCheck(t, gke, "private-nodes-without-private-google-access", "fail", 1)
	require.Equal(t, []string{"batch-private"}, witnessArgs(pga, "cluster"))
	key := requireCheck(t, gke, "secrets-key-unusable", "fail", 1)
	require.Equal(t, []string{"old-etcd-key"}, witnessArgs(key, "key"))

	r = runCLI(t, "policy", "evaluate", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var evaluated struct {
		Policies []struct {
			Policy       string `json:"policy"`
			Compliant    int    `json:"compliant"`
			Noncompliant int    `json:"noncompliant"`
			Unknown      int    `json:"unknown"`
			Provisional  int    `json:"provisional"`
		} `json:"policies"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &evaluated), r.Stdout)
	counts := map[string][3]int{}
	for _, p := range evaluated.Policies {
		counts[p.Policy] = [3]int{p.Compliant, p.Noncompliant, p.Unknown}
		require.Equal(t, p.Compliant+p.Noncompliant+p.Unknown, p.Provisional, "draft types make every result provisional")
	}
	require.Equal(t, map[string][3]int{
		"gcp.#AddressBestPractice":     {3, 2, 0},
		"gcp.#BucketBestPractice":      {1, 2, 0},
		"gcp.#FirewallBestPractice":    {5, 1, 0},
		"gcp.#GKEClusterBestPractice":  {2, 1, 0},
		"gcp.#KMSKeyBestPractice":      {4, 1, 0},
		"gcp.#SQLInstanceBestPractice": {1, 3, 0},
	}, counts)

	r = runCLI(t, "query", "policy_violation", "policy=gcp.#SQLInstanceBestPractice")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	for _, rule := range []string{"gcp-sql-backups", "gcp-sql-pitr", "gcp-sql-public-ip", "gcp-sql-world-authorized-network"} {
		require.Contains(t, r.Stdout, `"rule":"`+rule+`"`)
	}
	// Impact: what depends on the prod-vpc network.
	r = runCLI(t, "query", "gcp_depends_on_network", "network=projects/example-project/global/networks/prod-vpc")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Equal(t, map[string]int{
		"subnet": 3, "firewall": 6, "route": 2, "router": 1,
		"gke_cluster": 2, "sql_instance": 2, "dns_zone": 1,
	}, kindCounts(r.Stdout))

	// Policies declare no identity_fields: they are not family members.
	require.NotContains(t, r.Stderr, "projection disabled")
	r = runCLI(t, "doctor")
	require.NotContains(t, r.Stdout+r.Stderr, "Inconsistent identity_fields")

	// Sensitive values never reach storage; secret references are kept.
	r = runCLI(t, "list", "--schema", "gcp.#RunService", "--select", "spec.template.spec.containers[*].env", "--all", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "[REDACTED]")
	require.Contains(t, r.Stdout, "orders-db-password")
	require.NotContains(t, r.Stdout, "fixture-literal-not-a-real-token")
	r = runCLI(t, "list", "--schema", "gcp.#GKECluster", "--select", "nodeConfig.metadata", "--all", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.NotContains(t, r.Stdout, "fixture-startup-script")
	r = runCLI(t, "list", "--schema", "gcp.#ComputeInstance", "--select", "metadata.items", "--all", "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "kube-env", "metadata keys are kept")
	require.Contains(t, r.Stdout, "[REDACTED]")
	require.NotContains(t, r.Stdout, "fixture-kube-env-not-a-real-cert")
	require.NotContains(t, r.Stdout, "fixture-instance-startup-script")
	require.Contains(t, r.Stdout, "FIXTURE-OSLOGIN-TRUE", "allowlisted metadata values are kept")

	// One external-IP fact per address, on the network it is attached on.
	r = runCLI(t, "query", "gcp_instance_external_ip")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, `"address":"203.0.113.50"`)
	require.Contains(t, r.Stdout, `"network":"projects/example-project/global/networks/prod-vpc"`)
	require.Equal(t, 1, strings.Count(r.Stdout, `"address":`), r.Stdout)

	// Remove every noncompliant resource: each family passes.
	dropFixtureRecords(t, pudlDir, map[string][]string{
		"firewalls":     {"allow-ssh-world"},
		"addresses":     {"nat-ip-2", "spare-ip"},
		"buckets":       {"example-project-public-assets", "example-project-legacy-uploads"},
		"sql-instances": {"legacy-mysql", "reporting-db", "analytics-mysql"},
		"kms-keys":      {"legacy-key", "old-etcd-key"},
		"gke-clusters":  {"legacy-public", "batch-private"},
	})
	families = checkGCPPack(t, 0)
	for family, checks := range families {
		for name, c := range checks {
			require.Equal(t, "pass", c.Outcome, "%s %s", family, name)
		}
	}
}

// dropFixtureRecords rewrites installed fixtures without the named records
// (matched by name, or by the last segment of a fully qualified name), in the
// gcloud fixture and in the CAI fixture where the type has one (a CAI asset's
// name ends in the resource's name).
func dropFixtureRecords(t *testing.T, pudlDir string, drop map[string][]string) {
	t.Helper()
	for typ, names := range drop {
		dir := filepath.Join(pudlDir, "populators", "gcp")
		dropRecordsFrom(t, filepath.Join(dir, typ+".json"), typ, names)
		if cai := filepath.Join(dir, "cai", typ+".json"); fileExists(cai) {
			dropRecordsFrom(t, cai, typ, names)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dropRecordsFrom(t *testing.T, path, typ string, names []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var records []map[string]any
	require.NoError(t, json.Unmarshal(data, &records))
	var kept []map[string]any
	for _, rec := range records {
		name, _ := rec["name"].(string)
		short := name[strings.LastIndex(name, "/")+1:]
		dropped := false
		for _, n := range names {
			dropped = dropped || n == short
		}
		if !dropped {
			kept = append(kept, rec)
		}
	}
	require.Len(t, kept, len(records)-len(names), typ)
	out, err := json.Marshal(kept)
	require.NoError(t, err)
	writeFile(t, path, string(out))
}
