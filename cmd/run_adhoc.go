package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// adHocModel resolves a plugin from mu's local CAS and constructs the same
// model shape used by registered runs. The model is intentionally not written
// to the schema repository; only its run/snapshot artifacts are durable.
func adHocModel(spec string, inputArgs []string) (*systemmodel.SystemModel, string, string, error) {
	plugin, err := parsePluginSpec(spec)
	if err != nil {
		return nil, "", "", err
	}
	input, err := parseKeyValueInputs(inputArgs)
	if err != nil {
		return nil, "", "", err
	}
	def, err := cachedPluginDefinition(plugin)
	if err != nil {
		return nil, "", "", err
	}
	if info, infoErr := loadMuPluginInfo(plugin); infoErr == nil {
		if !hasCapability(info["capabilities"], "observe") {
			return nil, "", "", fmt.Errorf("mu plugin %q does not advertise observe capability", plugin)
		}
	}
	name := "adhoc-" + strings.ReplaceAll(plugin, "_", "-")
	return &systemmodel.SystemModel{
		Name:    name,
		Plugins: []systemmodel.PluginDef{def},
		Populate: systemmodel.Populate{
			Plugin:       plugin,
			Input:        input,
			Differential: false,
		},
	}, mustCurrentDir(), effectivePudlDir(), nil
}

func hasCapability(value any, want string) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func mustCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func cachedPluginDefinition(name string) (systemmodel.PluginDef, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return systemmodel.PluginDef{}, fmt.Errorf("locate mu plugin cache: %w", err)
	}
	dir := filepath.Join(home, ".mu", "plugins", name)
	bundles, err := filepath.Glob(filepath.Join(dir, "bundle-*"))
	if err != nil || len(bundles) == 0 {
		return systemmodel.PluginDef{}, fmt.Errorf("mu plugin %q is not cached under %s; run `mu plugin install %s` or use a registered model with plugins: {script: ...}", name, dir, name)
	}
	sort.Strings(bundles)
	base := filepath.Base(bundles[len(bundles)-1])
	return systemmodel.PluginDef{Name: name, Digest: "sha256:" + strings.TrimPrefix(base, "bundle-")}, nil
}

func createAdHocMuRoot() (string, func(), error) {
	dir, err := os.MkdirTemp("", "pudl_adhoc_mu_")
	if err != nil {
		return "", nil, fmt.Errorf("create ad-hoc mu workspace: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mu.cue"), []byte("package mu\n"), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("write ad-hoc mu workspace: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
