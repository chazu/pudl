package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chazu/pudl/internal/acute"
	"github.com/chazu/pudl/internal/mubridge"
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

// These aliases preserve the command package's report surface while keeping
// ACUTE lifecycle values independent of Cobra and subprocess adapters.
type ResourceDrift = acute.ResourceDrift
type ModelDriftResult = acute.ModelDriftResult

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
	// A differential observe always reads the live system, so the verdict is
	// verified regardless of outcome.
	return ModelDriftResult{Clean: len(drifted) == 0, Drifted: drifted, Verified: true}, nil
}

// renderReconcileMuCue emits a mu.cue with one converge-plugin target whose
// sources are the model's desired (rendered as manifests). The SAME target
// serves both `mu observe` (drift) and `mu build` (converge) — the §5.5 apply
// path. manifestSources are absolute paths because catalog-installed plugins
// run from their extracted bundle directory, not the project directory.
func renderReconcileMuCue(m *systemmodel.SystemModel, manifestSources []string) (string, error) {
	if !m.Convergent() {
		return "", fmt.Errorf("renderReconcileMuCue: model has no converge arm")
	}
	plugin := m.Converge.Plugin
	if _, ok := m.PluginByName(plugin); !ok {
		return "", fmt.Errorf("converge plugin %q is not declared in the model's plugins: block", plugin)
	}
	pluginsJSON, err := json.Marshal(m.Plugins)
	if err != nil {
		return "", fmt.Errorf("marshal plugins: %w", err)
	}
	srcJSON, err := json.Marshal(manifestSources)
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
	if err := renderWritableRefsPolicy(&b, len(m.Converge.SealedOutputs) > 0); err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "plugins: %s\n\n", pluginsJSON)
	b.WriteString("targets: [{\n")
	fmt.Fprintf(&b, "\ttarget:    %q\n", driftTargetName(m.Name))
	fmt.Fprintf(&b, "\ttoolchain: %q\n", plugin)
	fmt.Fprintf(&b, "\tsources:   %s\n", srcJSON)
	fmt.Fprintf(&b, "\tconfig:    %s\n", cfgJSON)
	if len(m.Converge.SealedInputs) > 0 || len(m.Converge.SealedOutputs) > 0 {
		b.WriteString("\tsealed_routing: \"strict\"\n")
	}
	if len(m.Converge.SealedInputs) > 0 {
		refs, modes, err := sealedInputProjection(m.Converge.SealedInputs)
		if err != nil {
			return "", fmt.Errorf("converge: %w", err)
		}
		sealedJSON, _ := json.Marshal(refs)
		fmt.Fprintf(&b, "\tsealed_inputs: %s\n", sealedJSON)
		modesJSON, _ := json.Marshal(modes)
		fmt.Fprintf(&b, "\tsealed_input_modes: %s\n", modesJSON)
	}
	if len(m.Converge.SealedOutputs) > 0 {
		refs, modes := sealedOutputProjection(m.Converge.SealedOutputs)
		sealedJSON, _ := json.Marshal(refs)
		fmt.Fprintf(&b, "\tsealed_outputs: %s\n", sealedJSON)
		modesJSON, _ := json.Marshal(modes)
		fmt.Fprintf(&b, "\tsealed_output_modes: %s\n", modesJSON)
	}
	b.WriteString("}]\n")
	return b.String(), nil
}

// writeDesiredManifests writes each desired entry as a JSON manifest file in dir
// (JSON is valid input for k8s' source parser) and returns their bare filenames
// (sources resolve relative to the mu.cue's dir, which is dir).
func writeDesiredManifests(desired []map[string]any, dir string) ([]string, error) {
	var names []string
	for i, d := range desired {
		data, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal desired[%d]: %w", i, err)
		}
		name := fmt.Sprintf("desired_%d.json", i)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return nil, fmt.Errorf("write desired[%d]: %w", i, err)
		}
		names = append(names, name)
	}
	return names, nil
}

// reconcileWorkspace is a prepared temp mu project (under muRoot) with the
// desired manifests + a converge-plugin target. Both observe (drift) and build
// (converge) run against Target. Call Cleanup when done.
type reconcileWorkspace struct {
	MuRoot string
	// Dir is the ephemeral generated-project directory. Exact-plan hashing
	// canonicalizes this path because a resume reconstructs equivalent files in
	// a fresh directory.
	Dir    string
	Target string
	// RunID tags the observations this workspace records, so a verdict can be
	// traced back to the run that produced it.
	RunID string
	// DryRun suppresses the observation write, since --dry-run promises no
	// catalog mutation.
	DryRun bool
	// Catalog is the run's handle, borrowed to record each observation. The
	// workspace does not own it and must not close it: the same handle outlives
	// every iteration of the converge loop that observes through here.
	Catalog *runCatalog
	// Mu is the subprocess seam: the workspace asks mu to observe, plan and apply
	// through it rather than reaching for exec.Command, so the whole converge
	// path can be driven by a scripted runner in a test.
	Mu      muRunner
	Cleanup func()
}

// setupReconcileWorkspace renders the desired manifests + mu.cue into a
// non-hidden temp subdir under muRoot (so mu merges it and inherits the project's
// toolchains/cache).
func setupReconcileWorkspace(cat *runCatalog, mu muRunner, m *systemmodel.SystemModel, muRoot, modelDir, runID string, dryRun bool) (*reconcileWorkspace, error) {
	if len(m.Desired) == 0 {
		return nil, fmt.Errorf("reconcile needs desired state; model %q declares none", m.Name)
	}
	rm := *m
	rm.Plugins = absolutizePlugins(m.Plugins, modelDir)
	projectLock, err := acquireMuProjectLock(muRoot)
	if err != nil {
		return nil, err
	}
	locked := true
	defer func() {
		if locked {
			_ = projectLock.Release()
		}
	}()

	// Collect any workspace an earlier run died holding, before adding ours.
	reportSweptWorkspaces(sweepStaleWorkspaces(muRoot, staleWorkspaceAge), !jsonOutput)

	dir, err := os.MkdirTemp(muRoot, workspacePrefix)
	if err != nil {
		return nil, fmt.Errorf("create reconcile workspace: %w", err)
	}
	// From here on the directory exists in the user's project, so every failure
	// path — and an interrupt — has to take it back out again.
	removeWorkspace := removeOnSignal(dir)
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			removeWorkspace()
			_ = projectLock.Release()
			locked = false
		})
	}
	names, err := writeDesiredManifests(m.Desired, dir)
	if err != nil {
		cleanup()
		return nil, err
	}
	manifestSources := make([]string, len(names))
	for i, name := range names {
		manifestSources[i] = filepath.Join(dir, name)
	}
	src, err := renderReconcileMuCue(&rm, manifestSources)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "mu.cue"), []byte(src), 0o644); err != nil {
		cleanup()
		return nil, fmt.Errorf("write reconcile mu.cue: %w", err)
	}
	locked = false
	return &reconcileWorkspace{
		MuRoot:  muRoot,
		Dir:     dir,
		Target:  driftTargetName(m.Name),
		RunID:   runID,
		DryRun:  dryRun,
		Catalog: cat,
		Mu:      mu,
		Cleanup: cleanup,
	}, nil
}

// observeDrift runs `mu observe` against the workspace target and interprets the
// differential result.
func (w *reconcileWorkspace) observeDrift() (ModelDriftResult, error) {
	stdout, err := w.Mu.Observe(filepath.Join(w.MuRoot, "mu.cue"), w.Target)
	if err != nil {
		return ModelDriftResult{}, err
	}
	res, err := interpretDifferentialObserve(stdout)
	if err != nil {
		return res, err
	}
	// Persist the observation this verdict came from. Without it a `clean` claim
	// rests on a value that only ever existed in memory, and the promotion it
	// drives cannot be audited afterwards.
	res.ObservationID = recordDriftObservation(w.Catalog, w.Target, w.RunID, w.DryRun, res, stdout)
	return res, nil
}

// recordDriftObservation stores a differential observation and returns its
// catalog entry ID, or "" when nothing was stored. Best-effort: failing to record
// the evidence must not fail a run that observed successfully, but it is reported
// and it leaves ObservationID empty rather than implying evidence exists.
//
// A dry run stores nothing, because --dry-run promises no catalog writes.
func recordDriftObservation(cat *runCatalog, target, runID string, dryRun bool, res ModelDriftResult, raw []byte) string {
	if dryRun {
		return ""
	}
	cfg, err := loadEffectiveConfig()
	if err != nil {
		if !jsonOutput {
			fmt.Printf("warning: could not load config to record the observation: %v\n", err)
		}
		return ""
	}
	db, err := cat.optional()
	if err != nil {
		if !jsonOutput {
			fmt.Printf("warning: could not open catalog to record the observation: %v\n", err)
		}
		return ""
	}

	drifted := make([]map[string]any, 0, len(res.Drifted))
	for _, d := range res.Drifted {
		drifted = append(drifted, map[string]any{
			"resource": d.Resource,
			"reason":   d.Reason,
			"diff":     d.Diff,
		})
	}
	id, err := mubridge.RecordDriftObservation(db, mubridge.DriftObservation{
		Target:       target,
		RunID:        runID,
		Clean:        res.Clean,
		DriftedCount: len(res.Drifted),
		Drifted:      drifted,
		Raw:          raw,
	}, cfg.DataPath)
	if err != nil {
		if !jsonOutput {
			fmt.Printf("warning: could not record the observation: %v\n", err)
		}
		return ""
	}
	return id
}

// runDrift is the read-only drift phase (observe-only on a convergent model):
// set up the workspace, observe once, report.
func runDrift(cat *runCatalog, mu muRunner, m *systemmodel.SystemModel, muRoot, modelDir, runID string) (ModelDriftResult, error) {
	w, err := setupReconcileWorkspace(cat, mu, m, muRoot, modelDir, runID, false)
	if err != nil {
		return ModelDriftResult{}, err
	}
	defer w.Cleanup()
	return w.observeDrift()
}
