// Package systemmodel defines the #SystemModel schema (pudl-owned) and loads a
// model *instance* into a Go struct that `pudl run` orchestrates from.
//
// A model instance is the run unit (one ACUTE loop); its `desired` entries are
// the per-status / --only unit. See docs/design/system-models/V1-BUILD-SPEC.md.
package systemmodel

import (
	_ "embed"
	"fmt"

	"cuelang.org/go/cue"
)

// schemaCUE is the canonical #SystemModel schema, compiled into every load so a
// model instance (`foo: #SystemModel & {...}`) resolves and is validated.
//
//go:embed schema.cue
var schemaCUE string

// SchemaCUE returns the canonical #SystemModel schema source (package
// systemmodel). The importer installs it into the schema repository so
// `pudl schema list` shows #SystemModel alongside the other built-in schemas.
func SchemaCUE() string { return schemaCUE }

// SystemModel is the decoded run unit. Orchestration-relevant fields only;
// `schema`/`relations` are carried by the CUE layer and decoded as needed.
type SystemModel struct {
	Name      string           `json:"name"`
	Plugins   []PluginDef      `json:"plugins,omitempty"`
	Populate  Populate         `json:"populate"`
	Checks    []Check          `json:"checks,omitempty"`
	Desired   []map[string]any `json:"desired,omitempty"`
	Converge  *PluginPlan      `json:"converge,omitempty"`
	Freshness *Freshness       `json:"freshness,omitempty"`
	// DependsOn names other #SystemModel instances this model depends on (by
	// their `name:`). Drives the model_depends_on facts + cross-model reasoning;
	// see docs/cross-model-dependencies.md.
	DependsOn []string `json:"depends_on,omitempty"`
}

// PluginDef is a plugin source declared in the model, mirroring mu's #PluginDef
// (one of script / digest / url+sha256). pudl emits these into the generated
// mu.cue so mu's resolver can find the plugin an arm references by name.
type PluginDef struct {
	Name    string   `json:"name"`
	Command []string `json:"command,omitempty"`
	Script  string   `json:"script,omitempty"`
	Digest  string   `json:"digest,omitempty"`
	URL     string   `json:"url,omitempty"`
	SHA256  string   `json:"sha256,omitempty"`
}

// PluginByName returns the declared plugin source for an arm's `plugin:` name.
func (m *SystemModel) PluginByName(name string) (PluginDef, bool) {
	for _, p := range m.Plugins {
		if p.Name == name {
			return p, true
		}
	}
	return PluginDef{}, false
}

// Populate carries both populate kinds (the #PluginObserve | #EweTarget union);
// Kind() reports which one this instance is, by which fields are set.
type Populate struct {
	// #PluginObserve
	Plugin       string         `json:"plugin,omitempty"`
	Input        map[string]any `json:"input,omitempty"`
	Differential bool           `json:"differential,omitempty"` // observer reports per-resource exists/matches (k8s); false → inventory set-diff
	// #EweTarget
	EweSource        string            `json:"eweSource,omitempty"`
	Outputs          []string          `json:"outputs,omitempty"`
	Network          bool              `json:"network,omitempty"`
	Impure           bool              `json:"impure,omitempty"`
	SealedInputs     map[string]string `json:"sealed_inputs,omitempty"`
	SealedInputModes map[string]string `json:"sealed_input_modes,omitempty"`
}

// PopulateKind enumerates the populate union arms.
type PopulateKind string

const (
	KindPluginObserve PopulateKind = "observe"
	KindEweTarget     PopulateKind = "ewe"
)

// Kind reports which populate arm this is. eweSource present → ewe; else observe.
func (p Populate) Kind() PopulateKind {
	if p.EweSource != "" {
		return KindEweTarget
	}
	return KindPluginObserve
}

// DifferentialDrift reports whether drift should be computed from a differential
// observe (the observer reads `desired` as sources and reports per-resource
// exists/matches, k8s-style) rather than an inventory set-diff. EweTarget populate
// always produces a record set → inventory; a #PluginObserve uses its
// `differential` field (default true). Observe-only models (no desired) never reach
// a drift path, so the value is moot there.
func (m *SystemModel) DifferentialDrift() bool {
	if m.Populate.Kind() == KindEweTarget {
		return false
	}
	return m.Populate.Differential
}

// PluginPlan is the converge arm: a declarative-apply plugin + its input.
type PluginPlan struct {
	Plugin string         `json:"plugin"`
	Input  map[string]any `json:"input,omitempty"`
}

// Check is an observe-only flag over a Datalog relation.
type Check struct {
	Name     string `json:"name"`
	Query    string `json:"query"`
	Expect   string `json:"expect"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Freshness is the loop cadence.
type Freshness struct {
	Every string `json:"every,omitempty"`
	Drift bool   `json:"drift,omitempty"`
}

// Convergent reports whether the model declares a convergence target (a converge
// arm). Observe-only models return false.
func (m *SystemModel) Convergent() bool {
	return m.Converge != nil && m.Converge.Plugin != ""
}

// DecodeValue validates and decodes a CUE value (a #SystemModel instance or a
// #SystemModel-derived definition) into a SystemModel. It re-extracts `desired`
// with hidden fields preserved so each record's `_schema` identity survives the
// Decode (which drops hidden fields). Used by LoadModel and the registry
// resolver.
func DecodeValue(inst cue.Value) (*SystemModel, error) {
	if err := inst.Validate(cue.Concrete(false)); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	var m SystemModel
	if err := inst.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("model has no name")
	}
	desired, err := decodeDesired(inst)
	if err != nil {
		return nil, fmt.Errorf("decode desired: %w", err)
	}
	if desired != nil {
		m.Desired = desired
	}
	return &m, nil
}

// decodeDesired extracts the `desired` list with hidden fields (e.g. `_schema`)
// preserved — Decode alone drops them.
func decodeDesired(inst cue.Value) ([]map[string]any, error) {
	dv := inst.LookupPath(cue.ParsePath("desired"))
	if !dv.Exists() {
		return nil, nil
	}
	iter, err := dv.List()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for iter.Next() {
		rec := map[string]any{}
		fields, err := iter.Value().Fields(cue.Hidden(true), cue.Optional(true))
		if err != nil {
			return nil, err
		}
		for fields.Next() {
			var val any
			if err := fields.Value().Decode(&val); err != nil {
				return nil, err
			}
			rec[fields.Selector().String()] = val
		}
		out = append(out, rec)
	}
	return out, nil
}
