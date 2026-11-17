package cmd

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// k8sWorkloadTypes are the workload collectors the posture family reads.
var k8sWorkloadTypes = []string{"deployments", "statefulsets", "daemonsets"}

// postureFindings lists a check's witnesses as "<kind> <namespace>/<name>
// <container>", with " init" for an init container and the missing limits
// when the witness has them, sorted.
func postureFindings(c packCheck) []string {
	out := []string{}
	for _, w := range c.Witnesses {
		a := w.Arguments
		s := a["kind"].(string) + " " + a["namespace"].(string) + "/" + a["name"].(string) + " " + a["container"].(string)
		if a["list"] == "initContainers" {
			s += " init"
		}
		if m, ok := a["missing"].(string); ok {
			s += ": " + m
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// The hostile fixtures' containers and init containers set no
// securityContext and no limits.
var k8sHostileContainers = []string{
	"DaemonSet hostile/hostile-daemonset app", "DaemonSet hostile/hostile-daemonset init init",
	"Deployment hostile/hostile-deployment app", "Deployment hostile/hostile-deployment init init",
	"StatefulSet hostile/hostile-statefulset app", "StatefulSet hostile/hostile-statefulset init init",
}

func withSuffix(items []string, suffix string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = s + suffix
	}
	return out
}

func sorted(items ...[]string) []string {
	var out []string
	for _, l := range items {
		out = append(out, l...)
	}
	sort.Strings(out)
	return out
}

// TestK8sPackPostureChecks runs the workload fixture collectors and the
// posture family: each check's findings are exactly the containers the
// fixtures plant for it, the hostile fixtures still store no planted value,
// and a waiver silences the one privileged container.
func TestK8sPackPostureChecks(t *testing.T) {
	pudlDir := cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	checks := checkPack(t, "k8s-fixture-", k8sWorkloadTypes, []string{"posture"}, 2)["posture"]
	require.Len(t, checks, 4)

	// Explicit privileged: false (checkout's proxy) passes.
	privileged := requireCheck(t, checks, "privileged-container", "fail", 1)
	require.Equal(t, "fail", privileged.Severity)
	require.Equal(t, []string{"DaemonSet logging/log-agent agent"}, postureFindings(privileged))
	require.NotEmpty(t, privileged.Witnesses[0].Arguments["resource_id"])

	// Passes: a version tag, a tag with a digest, a digest alone (checkout's
	// init container), :latest with a digest (edge-proxy).
	image := requireCheck(t, checks, "unpinned-image", "fail", 3)
	require.Equal(t, "warn", image.Severity)
	require.Equal(t, []string{
		"Deployment shop/catalog catalog",              // :latest
		"StatefulSet monitoring/metrics-store store",   // no tag
		"StatefulSet shop/cart-redis init-config init", // a registry port, no tag
	}, postureFindings(image))

	// Passes: checkout, edge-proxy, cart-redis's container and init container.
	limits := requireCheck(t, checks, "no-resource-limits", "fail", 11)
	require.Equal(t, "warn", limits.Severity)
	require.Equal(t, sorted(withSuffix(k8sHostileContainers, ": cpu and memory"), []string{
		"DaemonSet logging/log-agent agent: cpu and memory",
		"Deployment shop/catalog catalog: memory",
		"Deployment shop/checkout migrate init: cpu",
		"Deployment shop/checkout proxy: cpu and memory",
		"StatefulSet monitoring/metrics-store store: cpu and memory",
	}), postureFindings(limits))

	// Passes: pod runAsNonRoot (checkout, its init container and cart-redis's),
	// container runAsUser 10001 (catalog), pod runAsUser 65534
	// (metrics-store).
	root := requireCheck(t, checks, "may-run-as-root", "fail", 10)
	require.Equal(t, "warn", root.Severity)
	require.Equal(t, sorted(k8sHostileContainers, []string{
		"DaemonSet logging/log-agent agent", // nothing set
		"Deployment edge/edge-proxy proxy",  // pod runAsUser 0
		"Deployment shop/checkout proxy",    // container runAsUser 0 under pod runAsNonRoot
		"StatefulSet shop/cart-redis redis", // container runAsNonRoot: false under pod true
	}), postureFindings(root))

	requireNoPlanted(t, pudlDir, k8sPlanted...)

	// A waiver silences the privileged container: the only fail-severity
	// check is waived, so the command exits clean.
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "k8s_posture_waiver.cue"), `package models

#K8sFixturePostureChecks: waivers: [{
	check:   "privileged-container"
	witness: {namespace: "logging", name: "log-agent", container: "agent"}
	owner:   "platform", reason: "the log agent reads host logs"
	created: "2026-11-01", expires: "2099-12-31"
}]
`)
	checks = checkPack(t, "k8s-fixture-", k8sWorkloadTypes, []string{"posture"}, 0)["posture"]
	requireCheck(t, checks, "privileged-container", "waived", 1)
	requireCheck(t, checks, "unpinned-image", "fail", 3)
}

// TestK8sPackPostureChecksUnknown runs the posture families without their
// collectors: no observation, so every check is unknown. The live family
// after the live collectors is unknown too: they declare complete: false.
func TestK8sPackPostureChecksUnknown(t *testing.T) {
	useFakeKubectl(t)
	cliWorkspace(t)
	r := runCLI(t, "pack", "install", "k8s")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)

	requireUnknown := func(diagnostic string, models ...string) {
		t.Helper()
		r := runCLI(t, append(append([]string{"check"}, models...), "--json", "--detailed-exitcode")...)
		require.Equal(t, 1, exitCodeFor(r.Err), r.Stdout+r.Stderr)
		var report struct {
			Members []struct {
				Model  string `json:"model"`
				Report struct {
					Checks []struct {
						Name        string `json:"name"`
						Outcome     string `json:"outcome"`
						Diagnostics []struct {
							Code string `json:"code"`
						} `json:"diagnostics"`
					} `json:"checks"`
				} `json:"report"`
			} `json:"members"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report), r.Stdout)
		families := 0
		for _, m := range report.Members {
			if !strings.HasSuffix(m.Model, "posture-checks") {
				continue
			}
			families++
			require.Len(t, m.Report.Checks, 4, m.Model)
			for _, c := range m.Report.Checks {
				require.Equal(t, "unknown", c.Outcome, "%s %s", m.Model, c.Name)
				require.NotEmpty(t, c.Diagnostics)
				require.Equal(t, diagnostic, c.Diagnostics[0].Code, "%s %s", m.Model, c.Name)
			}
		}
		require.Equal(t, 1, families)
	}
	requireUnknown("missing_snapshot", "k8s-fixture-posture-checks")
	requireUnknown("missing_snapshot", "k8s-posture-checks")
	requireUnknown("incomplete_observation", "k8s-deployments", "k8s-statefulsets", "k8s-daemonsets", "k8s-posture-checks")
}
