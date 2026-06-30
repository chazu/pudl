//go:build smoke

package smoke

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmoke_K3sConvergence stands up a throwaway k3d cluster and validates the
// k8s convergence surface end-to-end PLUS cross-model dependencies on real
// converged resources: a `network` model (Namespace) and a `workloads` model
// (Deployment) whose namespace reference DERIVES an edge to network (no declared
// depends_on). Cleans up the cluster regardless of outcome.
func TestSmoke_K3sConvergence(t *testing.T) {
	requireTools(t, "docker", "k3d", "kubectl", "mu", "bb")
	requireDockerDaemon(t)

	src := muProjectSrc(t)
	plugin := filepath.Join(src, "plugins", "k8s", "plugin.bb")
	if _, err := exec.LookPath("bb"); err != nil {
		t.Skip("smoke: bb not found")
	}
	if !fileExists(plugin) {
		t.Skipf("smoke: k8s plugin not found at %s; skipping", plugin)
	}
	muRoot := minimalMuRoot(t, src)

	cluster := fmt.Sprintf("pudl-smoke-%d", pidish())
	kubeconfig := filepath.Join(t.TempDir(), "kube.yaml")
	ns := "pudl-smoke-ns"

	// Always tear the cluster down.
	t.Cleanup(func() {
		_ = exec.Command("k3d", "cluster", "delete", cluster).Run()
	})

	if out, err := exec.Command("k3d", "cluster", "create", cluster,
		"--no-lb", "--kubeconfig-update-default=false",
		"--kubeconfig-switch-context=false", "--wait", "--timeout", "180s").CombinedOutput(); err != nil {
		t.Fatalf("k3d cluster create: %v\n%s", err, out)
	}
	if out, err := exec.Command("k3d", "kubeconfig", "write", cluster, "--output", kubeconfig).CombinedOutput(); err != nil {
		t.Fatalf("k3d kubeconfig write: %v\n%s", err, out)
	}

	home := isolatedHome(t)
	pudlOK(t, home, "init")

	writeModel(t, home, "network.cue", fmt.Sprintf(`package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Network: sm.#SystemModel & {
	name: "network"
	plugins: [{name: "k8s", command: ["bb", %q]}]
	populate: {plugin: "k8s", differential: true, input: {kubeconfig: %q, kinds: ["Namespace"]}}
	desired: [{apiVersion: "v1", kind: "Namespace", metadata: name: %q}]
	converge: {plugin: "k8s", input: {kubeconfig: %q}}
}
`, plugin, kubeconfig, ns, kubeconfig))

	// NOTE: no depends_on — the edge must be DERIVED from metadata.namespace.
	writeModel(t, home, "workloads.cue", fmt.Sprintf(`package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Workloads: sm.#SystemModel & {
	name: "workloads"
	plugins: [{name: "k8s", command: ["bb", %q]}]
	populate: {plugin: "k8s", differential: true, input: {kubeconfig: %q, kinds: ["Deployment"]}}
	desired: [{
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {name: "nginx", namespace: %q}
		spec: {replicas: 1, selector: matchLabels: app: "nginx", template: {metadata: labels: app: "nginx", spec: containers: [{name: "nginx", image: "nginx:1.27-alpine"}]}}
	}]
	converge: {plugin: "k8s", input: {kubeconfig: %q, namespace: %q}}
}
`, plugin, kubeconfig, ns, kubeconfig, ns))

	// Derive the cross-model edge from the namespace reference.
	out := pudlOK(t, home, "model", "deps", "--derive")
	mustContain(t, out, "workloads depends on", "k3d derive")
	mustContain(t, out, "network", "k3d derive target")

	// Converge both on the real cluster.
	out = pudlOK(t, home, "run", "network", "--converge", "--mu-root", muRoot)
	mustContain(t, out, "outcome: clean", "network converge")
	out = pudlOK(t, home, "run", "workloads", "--converge", "--mu-root", muRoot)
	mustContain(t, out, "outcome: clean", "workloads converge")

	// The cluster actually has the resources.
	if out, err := kubectl(kubeconfig, "get", "ns", ns).CombinedOutput(); err != nil {
		t.Errorf("namespace %s not found: %v\n%s", ns, err, out)
	}
	if out, err := kubectl(kubeconfig, "get", "deploy", "-n", ns, "nginx").CombinedOutput(); err != nil {
		t.Errorf("deployment nginx not found: %v\n%s", err, out)
	}

	// Derived edge survives the real run reconcile (source separation).
	out = pudlOK(t, home, "query", "impacted_by", "changed=network")
	mustContain(t, out, "workloads", "k3d impacted_by after converge")

	out = pudlOK(t, home, "query", "--topo", "model_depends_on")
	if i, j := strings.Index(out, "network"), strings.Index(out, "workloads"); i < 0 || j < 0 || i > j {
		t.Errorf("topo order should list network before workloads:\n%s", out)
	}
}

func kubectl(kubeconfig string, args ...string) *exec.Cmd {
	return exec.Command("kubectl", append([]string{"--kubeconfig", kubeconfig}, args...)...)
}
