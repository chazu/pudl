package systemmodel

import (
	"encoding/json"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func compileTransform(t *testing.T, src string) *Transform {
	t.Helper()
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatal(err)
	}
	return &Transform{value: v}
}

func decodeRecord(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTransformUnwrapsAndKeepsNumbersExact(t *testing.T) {
	tr := compileTransform(t, `{in: _, out: in.versionedResources[0].resource & {assetName: in.name}}`)
	rec := decodeRecord(t, `{"name":"//x/a","versionedResources":[{"resource":{"id":12345678901234567890,"ratio":0.1,"big":1e400,"name":"a"}}]}`)
	out, keep, err := tr.Apply(rec)
	if err != nil || !keep {
		t.Fatalf("Apply: keep=%v err=%v", keep, err)
	}
	b, _ := json.Marshal(out)
	got := string(b)
	for _, want := range []string{`"id":12345678901234567890`, `"ratio":0.1`, `"assetName":"//x/a"`, `"name":"a"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("out %s lacks %s", got, want)
		}
	}
	if !strings.Contains(got, `"big":1e+400`) && !strings.Contains(got, `"big":1E+400`) && !strings.Contains(got, `"big":1e400`) {
		t.Fatalf("out %s lost the out-of-float64 number", got)
	}
}

func TestTransformNullDrops(t *testing.T) {
	tr := compileTransform(t, `{in: _, out: [if in.kind == "skip" {null}, in][0]}`)
	if _, keep, err := tr.Apply(decodeRecord(t, `{"kind":"skip"}`)); err != nil || keep {
		t.Fatalf("keep=%v err=%v, want a dropped record", keep, err)
	}
	if out, keep, err := tr.Apply(decodeRecord(t, `{"kind":"keep"}`)); err != nil || !keep || out["kind"] != "keep" {
		t.Fatalf("out=%v keep=%v err=%v", out, keep, err)
	}
}

func TestTransformErrors(t *testing.T) {
	for name, src := range map[string]string{
		"missing field": `{in: _, out: in.versionedResources[0].resource}`,
		"not an object": `{in: _, out: in.name}`,
		"not concrete":  `{in: _, out: {name: string}}`,
		"conflict":      `{in: _, out: in & {name: "other"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := compileTransform(t, src).Apply(decodeRecord(t, `{"name":"a"}`))
			if err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

const transformModel = `
_project: "example"
assets: #SystemModel & {
	name: "assets"
	populate: {
		each: {argv: ["echo", "[\"p1\"]"]}
		runs: [{
			argv: ["gcloud", "asset", "search-all-resources", "--scope=projects/{{item}}"]
			set: project: "{{item}}"
			transform: {
				in: _
				_r: in.versionedResources[0].resource
				out: [if in.assetType == "k8s.io/Pod" {null}, _r & {owner: _project}][0]
			}
		}, {argv: ["gcloud", "projects", "list", "--filter={{item}}"]}]
	}
}
`

func TestLoadModelReadsRunTransforms(t *testing.T) {
	m, err := loadModel([]byte(transformModel), "assets")
	if err != nil {
		t.Fatal(err)
	}
	if m.Populate.Runs[0].Transform == nil || m.Populate.Runs[1].Transform != nil {
		t.Fatal("only the first run declares a transform")
	}
	run := m.Populate.Runs[0].ForItem("p1")
	if run.Transform == nil {
		t.Fatal("ForItem must keep the transform")
	}
	out, keep, err := run.Transform.Apply(decodeRecord(t, `{"assetType":"compute.googleapis.com/Instance","versionedResources":[{"resource":{"name":"vm"}}]}`))
	if err != nil || !keep || out["name"] != "vm" || out["owner"] != "example" {
		t.Fatalf("out=%v keep=%v err=%v", out, keep, err)
	}
	if _, keep, err := run.Transform.Apply(decodeRecord(t, `{"assetType":"k8s.io/Pod"}`)); err != nil || keep {
		t.Fatalf("keep=%v err=%v, want the pod dropped", keep, err)
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "transform") {
		t.Fatalf("the recorded instance must not carry the transform: %s", encoded)
	}
}

func TestLoadModelRejectsTransformWithoutOut(t *testing.T) {
	src := `
m: #SystemModel & {
	name: "m"
	populate: runs: [{argv: ["true"], transform: {in: _}}]
}
`
	if _, err := loadModel([]byte(src), "m"); err == nil {
		t.Fatal("a transform without out must be rejected")
	}
}

func TestLoadModelRejectsUnknownTransformField(t *testing.T) {
	src := `
m: #SystemModel & {
	name: "m"
	populate: runs: [{argv: ["true"], transform: {in: _, out: in, extra: 1}}]
}
`
	if _, err := loadModel([]byte(src), "m"); err == nil {
		t.Fatal("transform is closed: an unknown field must be rejected")
	}
}
