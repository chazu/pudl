package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// driftTargetName is the mu target the drift phase observes (desired-as-sources).
func driftTargetName(modelName string) string {
	return fmt.Sprintf("//models/%s:drift", modelName)
}

// k8sObserveResource is one entry of a differential observer's current.resources
// (the k8s plugin: it reads desired manifests from sources and reports each
// resource's live status, mu/plugins/k8s/plugin.bb:351-372).
type k8sObserveResource struct {
	Resource string `json:"resource"` // "Kind/name"
	Exists   bool   `json:"exists"`
	Matches  bool   `json:"matches"` // only meaningful when Exists
	Diff     string `json:"diff,omitempty"`
}

// observeResultRaw matches mu's `observe --json` array element for a differential
// observer.
type observeResultRaw struct {
	Target  string `json:"target"`
	Current struct {
		Resources []k8sObserveResource `json:"resources"`
	} `json:"current"`
	Error string `json:"error,omitempty"`
}

// ResourceDrift is a single drifted resource and why.
type ResourceDrift struct {
	Resource string // "Kind/name"
	Reason   string // "missing" | "changed"
	Diff     string // present for "changed"
}

// ModelDriftResult is the instance-level drift verdict over a differential
// observe: clean iff every desired resource exists and matches.
type ModelDriftResult struct {
	Clean   bool
	Drifted []ResourceDrift
}

// interpretDifferentialObserve turns a differential observer's `observe --json`
// output (desired-as-sources -> per-resource exists/matches) into a model drift
// verdict. exists:false => missing (needs create); exists:true,matches:false =>
// changed (needs update). drift == ∅ iff all resources exist and match.
func interpretDifferentialObserve(observeJSON []byte) (ModelDriftResult, error) {
	var results []observeResultRaw
	if err := json.Unmarshal(observeJSON, &results); err != nil {
		return ModelDriftResult{}, fmt.Errorf("parse observe output: %w", err)
	}
	var drifted []ResourceDrift
	for _, r := range results {
		if r.Error != "" {
			return ModelDriftResult{}, fmt.Errorf("observe target %s: %s", r.Target, r.Error)
		}
		for _, res := range r.Current.Resources {
			switch {
			case !res.Exists:
				drifted = append(drifted, ResourceDrift{Resource: res.Resource, Reason: "missing"})
			case !res.Matches:
				drifted = append(drifted, ResourceDrift{Resource: res.Resource, Reason: "changed", Diff: res.Diff})
			}
		}
	}
	return ModelDriftResult{Clean: len(drifted) == 0, Drifted: drifted}, nil
}

// renderDriftMuCue emits a mu.cue that observes the converge plugin with the
// model's desired rendered to sources (the §5.5 apply path: pudl routes desired
// to the plugin, the plugin diffs vs live). manifestPaths are absolute.
func renderDriftMuCue(m *systemmodel.SystemModel, manifestPaths []string) (string, error) {
	if !m.Convergent() {
		return "", fmt.Errorf("renderDriftMuCue: model has no converge arm")
	}
	plugin := m.Converge.Plugin
	if _, ok := m.PluginByName(plugin); !ok {
		return "", fmt.Errorf("converge plugin %q is not declared in the model's plugins: block", plugin)
	}
	pluginsJSON, err := json.Marshal(m.Plugins)
	if err != nil {
		return "", fmt.Errorf("marshal plugins: %w", err)
	}
	srcJSON, err := json.Marshal(manifestPaths)
	if err != nil {
		return "", fmt.Errorf("marshal sources: %w", err)
	}
	cfgJSON := "{}"
	if len(m.Converge.Input) > 0 {
		b, err := json.Marshal(m.Converge.Input)
		if err != nil {
			return "", fmt.Errorf("marshal converge input: %w", err)
		}
		cfgJSON = string(b)
	}

	var b strings.Builder
	b.WriteString("package mu\n\n")
	fmt.Fprintf(&b, "plugins: %s\n\n", pluginsJSON)
	b.WriteString("targets: [{\n")
	fmt.Fprintf(&b, "\ttarget:    %q\n", driftTargetName(m.Name))
	fmt.Fprintf(&b, "\ttoolchain: %q\n", plugin)
	fmt.Fprintf(&b, "\tsources:   %s\n", srcJSON)
	fmt.Fprintf(&b, "\tconfig:    %s\n", cfgJSON)
	b.WriteString("}]\n")
	return b.String(), nil
}

// writeDesiredManifests writes each desired entry as a JSON manifest file in dir
// (JSON is valid input for k8s' source parser) and returns their absolute paths.
func writeDesiredManifests(desired []map[string]any, dir string) ([]string, error) {
	var paths []string
	for i, d := range desired {
		data, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal desired[%d]: %w", i, err)
		}
		p := filepath.Join(dir, fmt.Sprintf("desired_%d.json", i))
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, fmt.Errorf("write desired[%d]: %w", i, err)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// runDrift executes the drift phase for a convergent model: render desired to
// sources, observe the converge plugin (which diffs desired vs live), and
// interpret the result. Differential observers (k8s) do the diff; pudl reads the
// verdict. Returns the model-level drift result.
func runDrift(m *systemmodel.SystemModel, muRoot, modelDir string) (ModelDriftResult, error) {
	if len(m.Desired) == 0 {
		return ModelDriftResult{}, fmt.Errorf("drift needs desired state; model %q declares none", m.Name)
	}
	rm := *m
	rm.Plugins = absolutizePlugins(m.Plugins, modelDir)

	dir, err := os.MkdirTemp(muRoot, "pudl_run_")
	if err != nil {
		return ModelDriftResult{}, fmt.Errorf("create drift workspace: %w", err)
	}
	defer os.RemoveAll(dir)

	manifestPaths, err := writeDesiredManifests(m.Desired, dir)
	if err != nil {
		return ModelDriftResult{}, err
	}
	src, err := renderDriftMuCue(&rm, manifestPaths)
	if err != nil {
		return ModelDriftResult{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(src), 0o644); err != nil {
		return ModelDriftResult{}, fmt.Errorf("write drift mu.cue: %w", err)
	}

	target := driftTargetName(m.Name)
	cmd := exec.Command("mu", "observe", "--config", filepath.Join(muRoot, "mu.cue"), "--json", target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ModelDriftResult{}, fmt.Errorf("mu observe %s: %w: %s", target, err, strings.TrimSpace(stderr.String()))
	}
	return interpretDifferentialObserve(stdout.Bytes())
}
