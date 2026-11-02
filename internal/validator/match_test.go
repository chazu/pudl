package validator

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestDecodeMatch(t *testing.T) {
	const firewall = `{
	_pudl: schema_type: "base"
	kind: "compute#firewall" @pudl(match)
	name: string
	metadata: labels: "cloud.googleapis.com/location": string @pudl(match)
	optional?: string @pudl(match)
	...
}`
	var meta SchemaMetadata
	decodeMatch(cuecontext.New().CompileString(firewall), &meta)
	if meta.MatchError != "" || len(meta.Match) != 2 {
		t.Fatalf("want 2 rules (optional fields are not claims), got %v (error %q)", meta.Match, meta.MatchError)
	}
	labels := map[string]interface{}{"labels": map[string]interface{}{"cloud.googleapis.com/location": "us-central1"}}
	for name, tc := range map[string]struct {
		record map[string]interface{}
		want   bool
	}{
		"all match fields satisfied": {map[string]interface{}{"kind": "compute#firewall", "metadata": labels}, true},
		"other kind":                 {map[string]interface{}{"kind": "compute#route", "metadata": labels}, false},
		"nested field missing":       {map[string]interface{}{"kind": "compute#firewall"}, false},
		"wrong type":                 {map[string]interface{}{"kind": 1, "metadata": labels}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := meta.Claims(tc.record); got != tc.want {
				t.Fatalf("Claims = %v, want %v", got, tc.want)
			}
		})
	}

	var bad SchemaMetadata
	decodeMatch(cuecontext.New().CompileString(`{kind: "x" @pudl(match="yes")}`), &bad)
	if !strings.Contains(bad.MatchError, "match takes no value") || bad.Match != nil {
		t.Fatalf("want a match value error, got %q (rules %v)", bad.MatchError, bad.Match)
	}
}
