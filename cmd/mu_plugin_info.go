package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

// loadMuPluginInfo asks the installed mu CLI for the same discover contract it
// uses at runtime. PUDL treats discovery as optional metadata: a model can be
// described while offline, but when available the plugin's capabilities and
// config_schema are surfaced instead of being guessed in PUDL.
func loadMuPluginInfo(name string) (map[string]any, error) {
	cmd := exec.Command("mu", "plugin", "info", "-json", name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("mu plugin info %q: %s", name, stderr.String())
		}
		return nil, fmt.Errorf("mu plugin info %q: %w", name, err)
	}
	var info map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("decode mu plugin info %q: %w", name, err)
	}
	return info, nil
}
