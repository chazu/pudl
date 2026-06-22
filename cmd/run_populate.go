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
// populate arm: it passes the model's declared plugin source through (the
// model-level `plugins:` block) and adds a single target wired to that plugin's
// toolchain with the arm's input as config.
//
// Grounded: mu loads mu.cue only (mu/internal/config/loader.go:15), resolves the
// target's toolchain to a plugin via Config.Plugins (coordinator.Observe ->
// PluginResolver), then dispatches mgr.Observe(toolchain, ...). The plugin source
// comes from the model (self-contained; mirrors mu.cue), not pudl config. Only
// #PluginObserve is handled here; ewe populate is a later slice.
func renderPopulateMuCue(m *systemmodel.SystemModel) (string, error) {
	if m.Populate.Kind() != systemmodel.KindPluginObserve {
		return "", fmt.Errorf("renderPopulateMuCue: populate is %s, only %s supported in V1",
			m.Populate.Kind(), systemmodel.KindPluginObserve)
	}
	plugin := m.Populate.Plugin
	if plugin == "" {
		return "", fmt.Errorf("renderPopulateMuCue: populate has no plugin")
	}
	if _, ok := m.PluginByName(plugin); !ok {
		return "", fmt.Errorf("populate plugin %q is not declared in the model's plugins: block", plugin)
	}
	pluginsJSON, err := json.Marshal(m.Plugins)
	if err != nil {
		return "", fmt.Errorf("marshal plugins: %w", err)
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
	fmt.Fprintf(&b, "plugins: %s\n\n", pluginsJSON)
	b.WriteString("targets: [{\n")
	fmt.Fprintf(&b, "\ttarget:    %q\n", populateTargetName(m.Name))
	fmt.Fprintf(&b, "\ttoolchain: %q\n", plugin)
	fmt.Fprintf(&b, "\tconfig:    %s\n", cfgJSON)
	b.WriteString("}]\n")
	return b.String(), nil
}
