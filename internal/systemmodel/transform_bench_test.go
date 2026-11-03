package systemmodel

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

// caiInstance is a synthetic CAI search result shaped like a Compute instance
// (about 3 KB): nested objects, arrays and labels.
func caiInstance(i int) map[string]any {
	items := []any{}
	for k := 0; k < 6; k++ {
		items = append(items, map[string]any{"key": fmt.Sprintf("k%d", k), "value": "v"})
	}
	disks := []any{}
	for k := 0; k < 3; k++ {
		disks = append(disks, map[string]any{"boot": k == 0, "deviceName": fmt.Sprintf("disk-%d", k), "source": "https://www.googleapis.com/compute/v1/projects/p/zones/z/disks/d", "licenses": []any{"https://www.googleapis.com/compute/v1/projects/l/global/licenses/x"}})
	}
	resource := map[string]any{
		"id": fmt.Sprint(1000000 + i), "name": fmt.Sprintf("vm-%d", i), "status": "RUNNING",
		"selfLink":          fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/p/zones/z/instances/vm-%d", i),
		"machineType":       "https://www.googleapis.com/compute/v1/projects/p/zones/z/machineTypes/e2-standard-4",
		"labels":            map[string]any{"team": "orders", "env": "staging", "app": "api"},
		"metadata":          map[string]any{"kind": "compute#metadata", "items": items},
		"disks":             disks,
		"networkInterfaces": []any{map[string]any{"network": "https://www.googleapis.com/compute/v1/projects/p/global/networks/n", "subnetwork": "https://www.googleapis.com/compute/v1/projects/p/regions/r/subnetworks/s", "networkIP": "10.0.0.1", "accessConfigs": []any{map[string]any{"natIP": "203.0.113.1", "type": "ONE_TO_ONE_NAT"}}}},
		"serviceAccounts":   []any{map[string]any{"email": "sa@p.iam.gserviceaccount.com", "scopes": []any{"https://www.googleapis.com/auth/cloud-platform"}}},
		"scheduling":        map[string]any{"automaticRestart": true, "onHostMaintenance": "MIGRATE", "preemptible": false},
		"tags":              map[string]any{"items": []any{"web", "ssh"}},
	}
	return map[string]any{
		"name": fmt.Sprintf("//compute.googleapis.com/projects/p/zones/z/instances/vm-%d", i), "assetType": "compute.googleapis.com/Instance",
		"versionedResources": []any{map[string]any{"version": "v1", "resource": resource}},
	}
}

func benchTransform(b *testing.B, src string) {
	ctx := cuecontext.New()
	tr := &Transform{value: ctx.CompileString(src)}
	recs := make([]map[string]any, 5000)
	for i := range recs {
		recs[i] = caiInstance(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, r := range recs {
			if _, _, err := tr.Apply(r); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkTransformUnwrap5000(b *testing.B) {
	benchTransform(b, `{in: _, out: in.versionedResources[0].resource}`)
}

func BenchmarkTransformReshape5000(b *testing.B) {
	benchTransform(b, `{in: _, _r: in.versionedResources[0].resource, out: _r & {assetName: in.name, zone: _r.zone | *""}}`)
}
