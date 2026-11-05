package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/doctor"
	"github.com/chazu/pudl/internal/graphexport"
)

// Two schemas describe one cluster (a GKE cluster and a kube context with
// the same API server); a third type's records point at the context.
const sameResourceLabSchema = `package lab

#GKECluster: {
	_pudl: {
		schema_type:   "base"
		resource_type: "lab.gke_cluster"
		identity_fields: ["selfLink"]
		graph: display: {code: "GKE", label: "GKE cluster", class: "compute"}
		same_resource: [{kind: "k8s.cluster", key: host: "endpoint"}]
	}
	selfLink: string
	endpoint: string
	project:  string
	...
}

#KubeContext: {
	_pudl: {
		schema_type:   "base"
		resource_type: "lab.kube_context"
		identity_fields: ["context"]
		graph: display: {code: "KCTX", label: "Kube context", class: "compute"}
		same_resource: [{kind: "k8s.cluster", key: host: {path: "server", trim_prefix: "https://"}}]
	}
	context: string
	server:  string
	...
}

#Workload: {
	_pudl: {
		schema_type:   "base"
		resource_type: "lab.workload"
		identity_fields: ["name"]
		graph: display: {code: "WL", label: "Workload", class: "compute"}
		facts: lab_workload: args: {context: "context"}
	}
	name:    string
	context: string
	...
}
`

const sameResourceLabRules = `package rules

graph_edge_type_lab_runs_on: {
	head: {rel: "graph_edge_type", args: {type: "lab_runs_on", category: "dependency", label: "runs on", meaning: "Runs on this cluster."}}
}

lab_graph_workload_cluster: {
	head: {rel: "graph_edge", args: {type: "lab_runs_on", source: "$R", target: "$C", basis: "observed"}}
	body: [{rel: "lab_workload", args: {resource_id: "$R", context: "$C"}}]
}
`

func importLabRecord(t *testing.T, dir, name, schema, record string) {
	t.Helper()
	path := filepath.Join(dir, name+".json")
	require.NoError(t, os.WriteFile(path, []byte(record), 0o644))
	r := runCLI(t, "import", "--path", path, "--schema", schema, "--json")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
}

func TestSameResourceGroupsGraphShowAndDoctor(t *testing.T) {
	pudlDir := cliWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(pudlDir, "schema", "lab"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "schema", "lab", "lab.cue"), []byte(sameResourceLabSchema), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "schema", "pudl", "rules", "lab.cue"), []byte(sameResourceLabRules), 0o644))
	resetSchemaCaches()

	records := t.TempDir()
	importLabRecord(t, records, "gke", "lab.#GKECluster", `{"selfLink": "projects/p/locations/l/clusters/prod", "endpoint": "34.1.2.3", "project": "example-project"}`)
	importLabRecord(t, records, "ctx", "lab.#KubeContext", `{"context": "prod", "server": "https://34.1.2.3"}`)
	importLabRecord(t, records, "dev", "lab.#KubeContext", `{"context": "dev", "server": "https://127.0.0.1:6443"}`)
	importLabRecord(t, records, "web", "lab.#Workload", `{"name": "web", "context": "prod"}`)

	// Graph export: the GKE cluster and the prod context are one node.
	g := exportGraph(t)
	var cluster *graphexport.Resource
	types := map[string]int{}
	for i, r := range g.Resources {
		types[r.Type]++
		if len(r.Representations) > 0 {
			require.Nil(t, cluster, "one group")
			cluster = &g.Resources[i]
		}
	}
	require.NotNil(t, cluster, "%+v", g.Resources)
	require.Len(t, cluster.Representations, 2)
	require.Equal(t, "lab.#GKECluster", cluster.Representations[0].Schema, "labels come from the first schema by name")
	require.Equal(t, "lab.#KubeContext", cluster.Representations[1].Schema)
	require.Equal(t, "lab.gke_cluster", cluster.Type)
	require.Equal(t, map[string]int{"lab.gke_cluster": 1, "lab.kube_context": 1, "lab.workload": 1}, types,
		"the dev context stays its own node")

	// The workload's reference to the context resolves to the group.
	var runsOn []graphexport.Relationship
	for _, rel := range g.Relationships {
		if rel.Type == "lab_runs_on" {
			runsOn = append(runsOn, rel)
		}
	}
	require.Len(t, runsOn, 1)
	require.Equal(t, cluster.ID, runsOn[0].Target)

	// A member's own ID focuses its group.
	sub := graphexport.Subgraph(&g, []string{cluster.Representations[1].ID}, 1, nil)
	require.Len(t, sub.Resources, 2, "the group and the workload")

	// pudl show: each record names the other.
	r := runCLI(t, "show", cluster.Representations[1].EntryID, "--json", "--meta-only")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var shown showOutput
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &shown), r.Stdout)
	require.NotNil(t, shown.SameResource)
	require.Equal(t, cluster.ID, shown.SameResource.GroupID)
	require.Equal(t, []string{"k8s.cluster"}, shown.SameResource.Kinds)
	require.Len(t, shown.SameResource.Others, 1)
	require.Equal(t, "lab.#GKECluster", shown.SameResource.Others[0].Schema)

	r = runCLI(t, "show", cluster.Representations[0].EntryID, "--meta-only")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, "Same resource as (k8s.cluster):")
	require.Contains(t, r.Stdout, "lab.#KubeContext")

	// doctor: one group, no problems.
	check := doctor.CheckSameResource(workspaceSameResource(context.Background(), pudlDir))
	require.Equal(t, "ok", check.Status, "%s\n%s", check.Message, check.Details)
	require.Contains(t, check.Message, "1 group(s)")
}

// A second GKE cluster with the same endpoint (a recreated cluster whose old
// record is still current) would put two GKE records in one group: nothing
// is grouped, and both graph export and doctor say why.
func TestSameResourceConflictIsNotGrouped(t *testing.T) {
	pudlDir := cliWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(pudlDir, "schema", "lab"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pudlDir, "schema", "lab", "lab.cue"), []byte(sameResourceLabSchema), 0o644))
	resetSchemaCaches()

	records := t.TempDir()
	importLabRecord(t, records, "old", "lab.#GKECluster", `{"selfLink": "projects/p/locations/l/clusters/old", "endpoint": "34.1.2.3", "project": "example-project"}`)
	importLabRecord(t, records, "new", "lab.#GKECluster", `{"selfLink": "projects/p/locations/l/clusters/new", "endpoint": "34.1.2.3", "project": "example-project"}`)
	importLabRecord(t, records, "ctx", "lab.#KubeContext", `{"context": "prod", "server": "https://34.1.2.3"}`)

	g := exportGraph(t)
	for _, r := range g.Resources {
		require.Empty(t, r.Representations)
	}
	require.Len(t, g.Resources, 3)
	conflictWarned := false
	for _, w := range g.Meta.Warnings {
		if strings.Contains(w, "were not grouped") && strings.Contains(w, "2 records of lab.#GKECluster") {
			conflictWarned = true
		}
	}
	require.True(t, conflictWarned, "%v", g.Meta.Warnings)

	check := doctor.CheckSameResource(workspaceSameResource(context.Background(), pudlDir))
	require.Equal(t, "warning", check.Status)
	require.Contains(t, check.Details, "more than one record of lab.#GKECluster")
}
