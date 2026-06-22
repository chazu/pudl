package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chazu/pudl/internal/config"
	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/mubridge"
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

// absolutizePlugins resolves each plugin's relative `script` path against baseDir
// (the model file's directory). mu resolves a per-package mu.cue's plugin script
// relative to that package dir; emitting absolute paths makes the generated
// config location-independent (verified: relative paths resolve against the
// merged subdir, absolute paths just work).
func absolutizePlugins(plugins []systemmodel.PluginDef, baseDir string) []systemmodel.PluginDef {
	out := make([]systemmodel.PluginDef, len(plugins))
	for i, p := range plugins {
		if p.Script != "" && !filepath.IsAbs(p.Script) {
			p.Script = filepath.Join(baseDir, p.Script)
		}
		out[i] = p
	}
	return out
}

// findMuRoot walks up from startDir for a directory containing mu.cue (mu's
// project-root convention, mu/internal/config/loader.go:30).
func findMuRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, "mu.cue")); err == nil && !st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no mu.cue in %s or any parent (set --mu-root)", startDir)
		}
		dir = parent
	}
}

// runPopulate executes the populate phase: render a per-package mu.cue under the
// mu project root, run `mu observe --json` (inheriting the project's toolchains
// and cache), and ingest the result as catalog observe entries.
//
// muRoot is the mu project to run within (B: project-embedded). modelDir is the
// model file's directory, the base for resolving relative plugin scripts.
func runPopulate(m *systemmodel.SystemModel, muRoot, modelDir string) (*PopulateReport, error) {
	if m.Populate.Kind() == systemmodel.KindEweTarget {
		return runEwePopulate(m, muRoot, modelDir)
	}

	rm := *m
	rm.Plugins = absolutizePlugins(m.Plugins, modelDir)
	src, err := renderPopulateMuCue(&rm)
	if err != nil {
		return nil, err
	}

	// Non-hidden temp subdir under the project root so mergeSubdirConfigs picks
	// it up (it skips hidden dirs, mu/internal/config/loader.go:105).
	dir, err := os.MkdirTemp(muRoot, "pudl_run_")
	if err != nil {
		return nil, fmt.Errorf("create populate workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(src), 0o644); err != nil {
		return nil, fmt.Errorf("write populate mu.cue: %w", err)
	}

	target := populateTargetName(m.Name)
	cmd := exec.Command("mu", "observe", "--config", filepath.Join(muRoot, "mu.cue"), "--json", target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mu observe %s: %w: %s", target, err, strings.TrimSpace(stderr.String()))
	}

	count, err := ingestObserveOutput(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	return &PopulateReport{Target: target, Records: count}, nil
}

// renderEwePopulateMuCue emits a mu.cue project whose single target carries an
// inline plan that emits one `ewe`-body action (eweSource/outputs/network/
// impure). The populator program path is made relative to the mu project root so
// mu's plan-time CAS hashing (Resolve) can find it. Sealed inputs are declared
// at the target level; mu propagates them to the emitted action and resolves the
// refs, and the ewe sink reveals them only in-sink.
func renderEwePopulateMuCue(m *systemmodel.SystemModel, muRoot, modelDir string) (string, error) {
	p := m.Populate
	abs := p.EweSource
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(modelDir, p.EweSource)
	}
	rel, err := filepath.Rel(muRoot, abs)
	if err != nil {
		return "", fmt.Errorf("ewe_source path: %w", err)
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("ewe_source %q is outside the mu project root %q", p.EweSource, muRoot)
	}

	outputs := p.Outputs
	if len(outputs) == 0 {
		return "", fmt.Errorf("ewe populate: outputs must declare at least one records file")
	}

	action := map[string]any{
		"id":        "populate",
		"eweSource": rel,
		"outputs":   outputs,
		"network":   p.Network,
		"impure":    p.Impure,
	}
	actionJSON, err := json.Marshal(action)
	if err != nil {
		return "", fmt.Errorf("marshal ewe action: %w", err)
	}

	var b strings.Builder
	b.WriteString("package mu\n\n")
	b.WriteString("targets: [{\n")
	fmt.Fprintf(&b, "\ttarget: %q\n", populateTargetName(m.Name))
	if len(p.SealedInputs) > 0 {
		si, _ := json.Marshal(p.SealedInputs)
		fmt.Fprintf(&b, "\tsealed_inputs: %s\n", si)
	}
	if len(p.SealedInputModes) > 0 {
		sm, _ := json.Marshal(p.SealedInputModes)
		fmt.Fprintf(&b, "\tsealed_input_modes: %s\n", sm)
	}
	fmt.Fprintf(&b, "\tplan: [%s, \"action/emit\"]\n", actionJSON)
	b.WriteString("}]\n")
	return b.String(), nil
}

// runEwePopulate executes an #EweTarget populate arm: render a per-package
// mu.cue with an ewe-body action, run `mu build` (which runs the populator —
// HTTP fetch, in-sink secret reveal — and writes the declared records files to
// the project root), then wrap each records file as an ObserveResult and reuse
// the shipped catalog ingester. After ingest the catalog cannot tell an ewe
// record from a #PluginObserve one (ewe-populate-spec §3).
func runEwePopulate(m *systemmodel.SystemModel, muRoot, modelDir string) (*PopulateReport, error) {
	src, err := renderEwePopulateMuCue(m, muRoot, modelDir)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp(muRoot, "pudl_run_")
	if err != nil {
		return nil, fmt.Errorf("create populate workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(src), 0o644); err != nil {
		return nil, fmt.Errorf("write populate mu.cue: %w", err)
	}

	target := populateTargetName(m.Name)
	cmd := exec.Command("mu", "build", "--config", filepath.Join(muRoot, "mu.cue"), target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mu build %s: %w: %s", target, err, strings.TrimSpace(stderr.String()))
	}

	// Wrap each declared output (a JSON records array) as an ObserveResult and
	// feed the shipped ingester. The output lands in the action's WorkDir (the
	// project root) per mu's bare-output staging.
	var results []mubridge.ObserveResult
	for _, out := range m.Populate.Outputs {
		outPath := filepath.Join(muRoot, out)
		data, err := os.ReadFile(outPath)
		if err != nil {
			return nil, fmt.Errorf("read ewe output %q: %w", out, err)
		}
		_ = os.Remove(outPath) // don't leave build artifacts in the project root
		var arr []any
		if err := json.Unmarshal(data, &arr); err != nil {
			return nil, fmt.Errorf("ewe output %q is not a JSON records array: %w", out, err)
		}
		results = append(results, mubridge.ObserveResult{
			Target:  target,
			Current: map[string]any{"records": arr},
		})
	}

	wrapped, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("marshal observe results: %w", err)
	}
	count, err := ingestObserveOutput(wrapped)
	if err != nil {
		return nil, err
	}
	return &PopulateReport{Target: target, Records: count}, nil
}

// ingestObserveOutput feeds `mu observe --json` output into the catalog as
// observe entries, reusing the shipped IngestObserveResults (the same path
// `pudl mu ingest-observe` uses).
func ingestObserveOutput(observeJSON []byte) (int, error) {
	db, err := database.NewCatalogDB(config.GetPudlDir())
	if err != nil {
		return 0, fmt.Errorf("open catalog: %w", err)
	}
	defer db.Close()

	cfg, err := config.Load()
	if err != nil {
		return 0, fmt.Errorf("load config: %w", err)
	}
	inferrer, err := inference.NewSchemaInferrer(cfg.SchemaPath)
	if err != nil {
		return 0, fmt.Errorf("init schema inferrer: %w", err)
	}
	return mubridge.IngestObserveResults(db, bytes.NewReader(observeJSON), "pudl-run", cfg.DataPath, inferrer.GetInheritanceGraph())
}
