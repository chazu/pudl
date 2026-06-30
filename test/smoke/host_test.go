//go:build smoke

package smoke

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSmoke_DockerHostInventory uses a Docker container as a fake remote host to
// validate the INVENTORY convergence class (distinct from k8s differential) plus
// cross-model dependencies on it: observe the container via `docker exec`, ingest
// the records, set-diff drift against desired, and reason over a declared edge.
// No remote infra. Removes the container regardless of outcome.
func TestSmoke_DockerHostInventory(t *testing.T) {
	requireTools(t, "docker", "jq")
	requireDockerDaemon(t)

	ctr := fmt.Sprintf("pudl-smoke-host-%d", pidish())
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", ctr).Run()
	})

	// Start an alpine "host"; install curl (present), leave jq absent.
	if out, err := exec.Command("docker", "run", "-d", "--name", ctr, "alpine", "sleep", "900").CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	if out, err := exec.Command("docker", "exec", ctr, "apk", "add", "--no-cache", "curl").CombinedOutput(); err != nil {
		t.Fatalf("apk add curl: %v\n%s", err, out)
	}

	// Observe the container -> package names.
	pkgsOut, err := exec.Command("docker", "exec", ctr, "apk", "info").Output()
	if err != nil {
		t.Fatalf("apk info: %v", err)
	}
	var records []map[string]any
	for _, name := range strings.Fields(string(pkgsOut)) {
		records = append(records, map[string]any{
			"_schema": "pudl/linux.#Package", "host": "dockerhost", "name": name, "status": "present",
		})
	}
	if len(records) == 0 {
		t.Fatal("no packages observed in container")
	}
	observe := []map[string]any{{
		"target":  "//host/dockerhost",
		"current": map[string]any{"records": records},
	}}

	home := isolatedHome(t)
	pudlOK(t, home, "init")
	obsPath := filepath.Join(home, "observe.json")
	data, _ := json.Marshal(observe)
	if err := os.WriteFile(obsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	pudlOK(t, home, "mu", "ingest-observe", "--path", obsPath)

	// base-host: curl present -> the model is satisfied for curl.
	writeModel(t, home, "base.cue", `package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#BaseHost: sm.#SystemModel & {
	name: "base-host"
	plugins: [{name: "noop", command: ["true"]}]
	populate: {plugin: "noop", differential: false, input: {}}
	desired: [{_schema: "pudl/linux.#Package", host: "dockerhost", name: "curl", status: "present"}]
}
`)
	// app-host depends_on base-host (declared); wants jq, which is absent -> drift.
	writeModel(t, home, "app.cue", `package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#AppHost: sm.#SystemModel & {
	name:       "app-host"
	depends_on: ["base-host"]
	plugins: [{name: "noop", command: ["true"]}]
	populate: {plugin: "noop", differential: false, input: {}}
	desired: [
		{_schema: "pudl/linux.#Package", host: "dockerhost", name: "curl", status: "present"},
		{_schema: "pudl/linux.#Package", host: "dockerhost", name: "jq",   status: "present"},
	]
}
`)

	// base-host: curl is present -> clean.
	out := pudlOK(t, home, "run", "base-host", "--from-catalog")
	mustContain(t, out, "clean", "base-host inventory drift")

	// app-host: jq missing -> drift detected on the inventory class.
	out = pudlOK(t, home, "run", "app-host", "--from-catalog")
	mustContain(t, out, "jq", "app-host inventory drift (jq missing)")
	mustContain(t, out, "missing", "app-host drift reason")

	// Cross-model deps on the inventory class.
	out = pudlOK(t, home, "model", "deps")
	mustContain(t, out, "app-host depends on", "inventory declared graph")
	mustContain(t, out, "base-host", "inventory declared graph target")

	out = pudlOK(t, home, "query", "impacted_by", "changed=base-host")
	mustContain(t, out, "app-host", "inventory impacted_by")

	out = pudlOK(t, home, "query", "--topo", "model_depends_on")
	if i, j := strings.Index(out, "base-host"), strings.Index(out, "app-host"); i < 0 || j < 0 || i > j {
		t.Errorf("topo should list base-host before app-host:\n%s", out)
	}
}
