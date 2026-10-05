package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/proc"
)

// muPluginInfoTimeout bounds `mu plugin info`. Discovery is optional metadata,
// so a mu that hangs must not hang model inspection with it.
const muPluginInfoTimeout = 2 * time.Minute

// muPluginInfoFrom binds loadMuPluginInfo to ctx for model inspection.
func muPluginInfoFrom(ctx context.Context) muPluginDiscovery {
	return func(name string) (map[string]any, error) { return loadMuPluginInfo(ctx, name) }
}

// loadMuPluginInfo asks the installed mu CLI for the same discover contract it
// uses at runtime. PUDL treats discovery as optional metadata: a model can be
// described while offline, but when available the plugin's capabilities and
// config_schema are surfaced instead of being guessed in PUDL.
func loadMuPluginInfo(ctx context.Context, name string) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	out, err := proc.Output(ctx, muPluginInfoTimeout, "mu", "plugin", "info", "-json", name)
	if err != nil {
		return nil, fmt.Errorf("mu plugin info %q: %w", name, err)
	}
	var info map[string]any
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("decode mu plugin info %q: %w", name, err)
	}
	return info, nil
}
