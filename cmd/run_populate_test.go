package cmd

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

func TestRenderPopulateMuCue_K8s(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:    "k8s-policy",
		Plugins: []systemmodel.PluginDef{{Name: "k8s", Script: "/abs/plugins/k8s/plugin.bb"}},
		Populate: systemmodel.Populate{
			Plugin: "k8s",
			Input:  map[string]any{"namespace": "default", "context": "prod"},
		},
	}
	src, err := renderPopulateMuCue(m)
	require.NoError(t, err)

	// Must be valid CUE.
	ctx := cuecontext.New()
	v := ctx.CompileString(src, cue.Filename("mu.cue"))
	require.NoError(t, v.Err(), "generated mu.cue must compile:\n%s", src)

	// Plugin source passed through from the model's plugins: block.
	pluginName, err := v.LookupPath(cue.ParsePath("plugins[0].name")).String()
	require.NoError(t, err)
	assert.Equal(t, "k8s", pluginName)
	script, _ := v.LookupPath(cue.ParsePath("plugins[0].script")).String()
	assert.Equal(t, "/abs/plugins/k8s/plugin.bb", script)

	// Target wired to the toolchain with input as config.
	tgt, _ := v.LookupPath(cue.ParsePath("targets[0].target")).String()
	assert.Equal(t, "//models/k8s-policy:populate", tgt)
	tc, _ := v.LookupPath(cue.ParsePath("targets[0].toolchain")).String()
	assert.Equal(t, "k8s", tc)
	ns, _ := v.LookupPath(cue.ParsePath("targets[0].config.namespace")).String()
	assert.Equal(t, "default", ns)
}

func TestRenderPopulateMuCue_EmptyInput(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:     "m",
		Plugins:  []systemmodel.PluginDef{{Name: "host", Script: "p.bb"}},
		Populate: systemmodel.Populate{Plugin: "host"},
	}
	src, err := renderPopulateMuCue(m)
	require.NoError(t, err)
	v := cuecontext.New().CompileString(src, cue.Filename("mu.cue"))
	require.NoError(t, v.Err())
}

func TestRenderPopulateMuCue_UndeclaredPlugin(t *testing.T) {
	// arm references a plugin not in the plugins: block -> error.
	m := &systemmodel.SystemModel{
		Name:     "m",
		Populate: systemmodel.Populate{Plugin: "k8s"},
	}
	_, err := renderPopulateMuCue(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not declared")
}

func TestRenderPopulateMuCue_RejectsEwe(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:     "m",
		Populate: systemmodel.Populate{EweSource: "populate.cue"},
	}
	_, err := renderPopulateMuCue(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ewe")
}
