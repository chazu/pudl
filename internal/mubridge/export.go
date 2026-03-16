package mubridge

import (
	"strings"

	"pudl/internal/drift"
)

// MuConfig represents a mu.json configuration file.
type MuConfig struct {
	Plugins []PluginDef `json:"plugins,omitempty"`
	Targets []Target    `json:"targets,omitempty"`
}

// PluginDef matches mu's plugin definition format.
type PluginDef struct {
	Name    string   `json:"name"`
	Command []string `json:"command,omitempty"`
	Script  string   `json:"script,omitempty"`
}

// Target matches mu's target format.
type Target struct {
	Name      string         `json:"target"`
	Toolchain string         `json:"toolchain"`
	Sources   []string       `json:"sources,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
}

// ToolchainMapping maps a pudl schema reference prefix to a mu toolchain name.
// For example, "ec2" -> "aws", "k8s" -> "k8s", "file" -> "file".
type ToolchainMapping struct {
	Prefix    string // schema ref prefix to match (e.g. "ec2", "k8s")
	Toolchain string // mu toolchain name (e.g. "aws", "k8s", "file")
}

// DefaultMappings provides reasonable defaults for common resource types.
var DefaultMappings = []ToolchainMapping{
	{Prefix: "ec2", Toolchain: "aws"},
	{Prefix: "s3", Toolchain: "aws"},
	{Prefix: "iam", Toolchain: "aws"},
	{Prefix: "aws", Toolchain: "aws"},
	{Prefix: "k8s", Toolchain: "k8s"},
	{Prefix: "kubernetes", Toolchain: "k8s"},
	{Prefix: "file", Toolchain: "file"},
	{Prefix: "config", Toolchain: "file"},
}

// resolveToolchain maps a schema reference to a mu toolchain name.
// SchemaRef is like "ec2.#Instance" or "k8s.#Deployment".
// Returns the toolchain name, or "generic" as a fallback.
func resolveToolchain(schemaRef string, mappings []ToolchainMapping) string {
	lower := strings.ToLower(schemaRef)
	for _, m := range mappings {
		if strings.HasPrefix(lower, strings.ToLower(m.Prefix)) {
			return m.Toolchain
		}
	}
	return "generic"
}

// ExportMuConfig converts one or more drift results into a mu.json configuration.
// Each drifted definition becomes a target whose config is the desired state
// (DeclaredKeys from the drift result). The toolchain is inferred from the
// schema reference.
func ExportMuConfig(results []*DriftInput, mappings []ToolchainMapping) *MuConfig {
	if mappings == nil {
		mappings = DefaultMappings
	}

	// Collect unique toolchains needed.
	toolchainsSeen := map[string]bool{}
	var targets []Target

	for _, input := range results {
		if input.Result.Status == "clean" {
			continue // No drift, no target needed.
		}

		toolchain := resolveToolchain(input.SchemaRef, mappings)
		toolchainsSeen[toolchain] = true

		// Build target config from declared state.
		config := make(map[string]any, len(input.Result.DeclaredKeys))
		for k, v := range input.Result.DeclaredKeys {
			config[k] = v
		}

		targets = append(targets, Target{
			Name:      "//" + input.Result.Definition,
			Toolchain: toolchain,
			Sources:   input.Sources,
			Config:    config,
		})
	}

	cfg := &MuConfig{
		Targets: targets,
	}

	return cfg
}

// DriftInput pairs a drift result with metadata needed for mu config generation.
type DriftInput struct {
	Result    *drift.DriftResult
	SchemaRef string   // e.g. "ec2.#Instance"
	Sources   []string // source files (e.g. CUE definition files)
}

// --- Legacy types kept for backward compatibility ---

// ActionSpec matches mu's plugin protocol format.
// Deprecated: Use ExportMuConfig instead.
type ActionSpec struct {
	ID        string            `json:"id"`
	Command   []string          `json:"command"`
	Inputs    map[string]string `json:"inputs"`
	Outputs   []string          `json:"outputs"`
	DependsOn []string          `json:"depends_on,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Network   bool              `json:"network,omitempty"`
}

// PlanResponse matches mu's plan response format.
// Deprecated: Use ExportMuConfig instead.
type PlanResponse struct {
	Actions []ActionSpec      `json:"actions"`
	Outputs map[string]string `json:"outputs,omitempty"`
	Error   string            `json:"error,omitempty"`
}
