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
func runPopulate(m *systemmodel.SystemModel, muRoot, modelDir, pudlRoot string) (*PopulateReport, error) {
	if m.Populate.Kind() == systemmodel.KindEweTarget {
		// Self-staged; no external mu root needed (works for project + global).
		return runEwePopulate(m, modelDir, pudlRoot)
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

// renderEwePopulateMuCue emits a standalone mu.cue project (written at the root
// of a staged temp dir) whose single target carries an inline plan that emits
// one `ewe`-body action. eweSourceName is the populator file's name within that
// staged root (copied there by runEwePopulate), so it resolves regardless of
// where the model is registered (project or global ~/.pudl). Sealed inputs are
// declared at the target level; mu propagates them to the emitted action and
// resolves the refs, and the ewe sink reveals them only in-sink.
func renderEwePopulateMuCue(m *systemmodel.SystemModel, modelDir, eweSourceName string) (string, error) {
	p := m.Populate

	outputs := p.Outputs
	if len(outputs) == 0 {
		return "", fmt.Errorf("ewe populate: outputs must declare at least one records file")
	}

	action := map[string]any{
		"id":        "populate",
		"eweSource": eweSourceName,
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
	// Emit the model's plugins block (paths absolutized) so secret-provider
	// plugins are available to resolve sealed-input refs (env:/pass:/sops:).
	if len(m.Plugins) > 0 {
		pluginsJSON, err := json.Marshal(absolutizePlugins(m.Plugins, modelDir))
		if err != nil {
			return "", fmt.Errorf("marshal plugins: %w", err)
		}
		fmt.Fprintf(&b, "plugins: %s\n\n", pluginsJSON)
	}
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

// resolveEweSource locates a model's populator program. eweSource is, in order:
// an absolute path; a path under the owning pudl repo's populators/ dir
// (e.g. "github/populate.cue" -> <pudlRoot>/populators/github/populate.cue);
// a path relative to the pudl root (covers a "populators/..."-prefixed value);
// or a path relative to the model's own directory (co-located fallback).
func resolveEweSource(eweSource, modelDir, pudlRoot string) (string, error) {
	if eweSource == "" {
		return "", fmt.Errorf("ewe populate: eweSource is empty")
	}
	if filepath.IsAbs(eweSource) {
		if _, err := os.Stat(eweSource); err != nil {
			return "", fmt.Errorf("eweSource %q: %w", eweSource, err)
		}
		return eweSource, nil
	}
	var candidates []string
	if pudlRoot != "" {
		candidates = append(candidates,
			filepath.Join(pudlRoot, "populators", eweSource),
			filepath.Join(pudlRoot, eweSource),
		)
	}
	if modelDir != "" {
		candidates = append(candidates, filepath.Join(modelDir, eweSource))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("eweSource %q not found (looked in: %s)", eweSource, strings.Join(candidates, ", "))
}

// runEwePopulate executes an #EweTarget populate arm in a self-contained,
// staged mu project (so it works whether the model is registered in a project
// .pudl/schema or in the global ~/.pudl — neither needs a pre-existing mu
// project). It stages a temp dir as the mu root, copies the populator program
// in, runs `mu build` (HTTP fetch + in-sink secret reveal -> records files),
// then wraps each records file as an ObserveResult and reuses the shipped
// catalog ingester. After ingest the catalog cannot tell an ewe record from a
// #PluginObserve one (ewe-populate-spec §3). modelDir is the directory the model
// schema was loaded from (the base for resolving the eweSource + relative plugin
// scripts).
func runEwePopulate(m *systemmodel.SystemModel, modelDir, pudlRoot string) (*PopulateReport, error) {
	srcPath, err := resolveEweSource(m.Populate.EweSource, modelDir, pudlRoot)
	if err != nil {
		return nil, err
	}
	eweName := filepath.Base(srcPath)

	// Stage a standalone mu project: mu.cue (root config) + the populator file.
	dir, err := os.MkdirTemp("", "pudl_ewe_")
	if err != nil {
		return nil, fmt.Errorf("create populate workspace: %w", err)
	}
	defer os.RemoveAll(dir)

	progData, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("read eweSource %q: %w", m.Populate.EweSource, err)
	}
	if err := os.WriteFile(filepath.Join(dir, eweName), progData, 0o644); err != nil {
		return nil, fmt.Errorf("stage eweSource: %w", err)
	}

	src, err := renderEwePopulateMuCue(m, modelDir, eweName)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(src), 0o644); err != nil {
		return nil, fmt.Errorf("write populate mu.cue: %w", err)
	}

	target := populateTargetName(m.Name)
	cmd := exec.Command("mu", "build", "--config", filepath.Join(dir, "mu.cue"), target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mu build %s: %w: %s", target, err, strings.TrimSpace(stderr.String()))
	}

	// Wrap each declared output (a JSON records array) as an ObserveResult and
	// feed the shipped ingester. Outputs land in the action's WorkDir (the staged
	// project root) per mu's bare-output staging.
	var results []mubridge.ObserveResult
	for _, out := range m.Populate.Outputs {
		outPath := filepath.Join(dir, out)
		data, err := os.ReadFile(outPath)
		if err != nil {
			return nil, fmt.Errorf("read ewe output %q: %w", out, err)
		}
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
