package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// populateTargetName is the mu target a model's populate phase observes.
func populateTargetName(modelName string) string {
	return fmt.Sprintf("//models/%s:populate", modelName)
}

// renderPopulateMuCue emits a mu.cue project that observes a #PluginObserve
// populate arm: it declares the plugin (form-2 local .bb #PluginDef) and a single
// target wired to that plugin's toolchain with the arm's input as config.
//
// Grounded: mu loads mu.cue only (mu/internal/config/loader.go:15), resolves the
// target's toolchain to a plugin via Config.Plugins (coordinator.Observe ->
// PluginResolver), then dispatches mgr.Observe(toolchain, ...). pluginScript is
// the path to the plugin's .bb (resolved relative to the mu.cue dir by the
// resolver). Only #PluginObserve is handled here; ewe populate is a later slice.
func renderPopulateMuCue(m *systemmodel.SystemModel, pluginScript string) (string, error) {
	if m.Populate.Kind() != systemmodel.KindPluginObserve {
		return "", fmt.Errorf("renderPopulateMuCue: populate is %s, only %s supported in V1",
			m.Populate.Kind(), systemmodel.KindPluginObserve)
	}
	plugin := m.Populate.Plugin
	if plugin == "" {
		return "", fmt.Errorf("renderPopulateMuCue: populate has no plugin")
	}

	// The arm's input becomes the target config. JSON is valid CUE, so marshal
	// the input map and embed it as the config value.
	cfgJSON := "{}"
	if len(m.Populate.Input) > 0 {
		b, err := json.Marshal(m.Populate.Input)
		if err != nil {
			return "", fmt.Errorf("marshal populate input: %w", err)
		}
		cfgJSON = string(b)
	}

	var b strings.Builder
	b.WriteString("package mu\n\n")
	fmt.Fprintf(&b, "plugins: [{name: %q, script: %q}]\n\n", plugin, pluginScript)
	b.WriteString("targets: [{\n")
	fmt.Fprintf(&b, "\ttarget:    %q\n", populateTargetName(m.Name))
	fmt.Fprintf(&b, "\ttoolchain: %q\n", plugin)
	fmt.Fprintf(&b, "\tconfig:    %s\n", cfgJSON)
	b.WriteString("}]\n")
	return b.String(), nil
}
