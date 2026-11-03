package systemmodel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fallbackModel = `
assets: #SystemModel & {
	name: "assets"
	populate: {
		schema: "pudl/gcp.#Instance"
		each: {argv: ["gcloud", "organizations", "list", "--format=json"], select: "name"}
		runs: [{
			argv: ["gcloud", "asset", "search-all-resources", "--scope={{item}}"]
			transform: {in: _, out: in.versionedResources[0].resource}
		}]
		fallback: {
			when: ["SERVICE_DISABLED", "USER_PROJECT_DENIED"]
			observe: {
				timeout: "5m"
				each: {argv: ["gcloud", "projects", "list", "--format=json"], select: "projectId"}
				empty_when: ["SERVICE_DISABLED"]
				runs: [{
					argv: ["gcloud", "compute", "instances", "list", "--project={{item}}"]
					set: project: "{{item}}"
					transform: {in: _, out: in & {source: "gcloud"}}
				}]
			}
		}
	}
}
`

func TestLoadModelReadsFallback(t *testing.T) {
	m, err := loadModel([]byte(fallbackModel), "assets")
	require.NoError(t, err)
	f := m.Populate.Fallback
	require.NotNil(t, f)
	assert.Equal(t, []string{"SERVICE_DISABLED", "USER_PROJECT_DENIED"}, f.When)
	assert.Equal(t, "5m", f.Observe.Timeout)
	require.NotNil(t, f.Observe.Each)
	require.Len(t, f.Observe.Runs, 1)
	require.NotNil(t, m.Populate.Runs[0].Transform)
	require.NotNil(t, f.Observe.Runs[0].Transform, "fallback runs get their transforms")
	out, keep, err := f.Observe.Runs[0].Transform.Apply(map[string]any{"name": "vm"})
	require.NoError(t, err)
	assert.True(t, keep)
	assert.Equal(t, "gcloud", out["source"])

	template, err := NewTemplate(compileTemplate(t, fallbackModel, "assets"), TemplateOrigin{})
	require.NoError(t, err)
	_, err = template.Model()
	require.NoError(t, err, "fallback transforms are exempt from concreteness too")
}

func TestLoadModelRejectsBadFallbacks(t *testing.T) {
	base := func(fallback string) string {
		return `m: #SystemModel & {name: "m", populate: {runs: [{argv: ["true"]}], fallback: ` + fallback + `}}`
	}
	for name, tc := range map[string]struct{ src, want string }{
		"no patterns":        {base(`{when: [], observe: runs: [{argv: ["true"]}]}`), ""},
		"bad pattern":        {base(`{when: ["("], observe: runs: [{argv: ["true"]}]}`), "not a valid regular expression"},
		"nested fallback":    {base(`{when: ["x"], observe: {runs: [{argv: ["true"]}], fallback: {when: ["y"], observe: runs: [{argv: ["true"]}]}}}`), ""},
		"schema in fallback": {base(`{when: ["x"], observe: {runs: [{argv: ["true"]}], schema: "a.#B"}}`), ""},
		"item without each":  {base(`{when: ["x"], observe: runs: [{argv: ["echo", "{{item}}"]}]}`), "fallback.observe"},
		"each without item":  {base(`{when: ["x"], observe: {each: {argv: ["echo", "[]"]}, runs: [{argv: ["true"]}]}}`), "fallback.observe"},
		"bad empty_when":     {base(`{when: ["x"], observe: {empty_when: ["("], runs: [{argv: ["true"]}]}}`), "fallback.observe"},
		"no out":             {base(`{when: ["x"], observe: runs: [{argv: ["true"], transform: {in: _}}]}`), "out is required"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadModel([]byte(tc.src), "m")
			require.Error(t, err)
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q lacks %q", err, tc.want)
			}
		})
	}
}
