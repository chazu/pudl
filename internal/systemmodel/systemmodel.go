// Package systemmodel defines the #SystemModel schema (pudl-owned) and loads a
// model *instance* into a Go struct that `pudl check` orchestrates from.
//
// A model instance is the run unit: populate, drift, checks, report. See
// docs/design/system-models/V1-BUILD-SPEC.md (historical).
package systemmodel

import (
	_ "embed"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/waiver"
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
	Observation *Observation `json:"observation,omitempty"`
	Name        string       `json:"name"`
	Populate    Populate     `json:"populate,omitzero"`
	Checks      []Check      `json:"checks,omitempty"`
	// Waivers are accepted, time-boxed exceptions to the checks' findings.
	Waivers   []waiver.Waiver  `json:"waivers,omitempty"`
	Desired   []map[string]any `json:"desired,omitempty"`
	Freshness *Freshness       `json:"freshness,omitempty"`
	// DependsOn names other #SystemModel instances this model depends on (by
	// their `name:`). Drives the model_depends_on facts + cross-model reasoning;
	// see docs/cross-model-dependencies.md.
	DependsOn []string `json:"depends_on,omitempty"`
}

// Populate is the #CommandObserve arm. Absent marks a checks-only model.
type Populate struct {
	AllowSchemaFallback bool         `json:"allow_schema_fallback,omitempty"`
	Runs                []CommandRun `json:"runs,omitempty"`
	Schema              string       `json:"schema,omitempty"`
	Timeout             string       `json:"timeout,omitempty"`
	// Each enumerates the inputs every run is repeated for; nil runs Runs as
	// declared.
	Each *Enumeration `json:"each,omitempty"`
	// FailFast fails the populate phase at the first failing run, so nothing
	// is ingested; otherwise a failing run is a failed input.
	FailFast bool `json:"fail_fast,omitempty"`
	// EmptyWhen are regular expressions matched against the stderr of a run
	// that exited non-zero. A match means the input has no resources (an API
	// that is disabled, say): the run is an empty input, not a failed one.
	EmptyWhen []string `json:"empty_when,omitempty"`
	// FailWhen are regular expressions matched against the stderr of a run
	// that exited 0. A match means the command could not see everything
	// (gcloud's aggregated lists warn and exit 0 when a project refuses
	// them): the run is a failed input and its records are discarded.
	FailWhen []string `json:"fail_when,omitempty"`
	// Fallback observes the same resources another way when these runs
	// cannot (see Fallback).
	Fallback *Fallback `json:"fallback,omitempty"`
	// Absent: the model declares no populate arm (checks-only).
	Absent bool `json:"-"`
}

// CommandRun is one #CommandRun: a command whose stdout is JSON records.
type CommandRun struct {
	Argv []string       `json:"argv"`
	Set  map[string]any `json:"set,omitempty"`
	Dir  string         `json:"dir,omitempty"`
	// Transform is read from the CUE value (decodeTransforms), never from
	// JSON, and is not part of the recorded model instance.
	Transform *Transform `json:"-"`
}

// PopulateKind names the populate arm a model declares.
type PopulateKind string

const (
	KindCommand PopulateKind = "command"
	KindNone    PopulateKind = "none" // no populate arm: checks-only
)

// Kind reports the populate arm: none for a checks-only model, else command.
func (p Populate) Kind() PopulateKind {
	if p.Absent {
		return KindNone
	}
	return KindCommand
}

// IsZero reports that the model declares no populate arm, so JSON encoding
// omits the field (omitzero). An emitted `populate: {}` would not satisfy
// #CommandObserve, which requires runs.
func (p Populate) IsZero() bool {
	return p.Absent || (len(p.Runs) == 0 && p.Schema == "" && p.Timeout == "" && !p.AllowSchemaFallback && p.Each == nil && !p.FailFast && len(p.EmptyWhen) == 0 && len(p.FailWhen) == 0)
}

// Check is an observe-only flag over a Datalog relation.
type Check struct {
	Evidence []string `json:"evidence,omitempty"`
	MaxAge   string   `json:"max_age,omitempty"`
	Name     string   `json:"name"`
	Query    string   `json:"query"`
	Expect   string   `json:"expect"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
}

// Freshness is the loop cadence.
type Freshness struct {
	Every string `json:"every,omitempty"`
	Drift bool   `json:"drift,omitempty"`
}

// DecodeValue validates and decodes a CUE value (a #SystemModel instance or a
// #SystemModel-derived definition) into a SystemModel. It re-extracts `desired`
// with hidden fields preserved so each record's `_schema` identity survives the
// Decode (which drops hidden fields). Used by LoadModel and the registry
// resolver.
func DecodeValue(inst cue.Value) (*SystemModel, error) {
	if err := rejectRetiredFields(inst); err != nil {
		return nil, err
	}
	if err := inst.Validate(cue.Concrete(false)); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	// Waivers must be complete: a missing owner or date fails here, naming
	// the field, rather than as an opaque decode error.
	if w := inst.LookupPath(cue.ParsePath("waivers")); w.Exists() {
		if err := w.Validate(cue.Concrete(true)); err != nil {
			return nil, fmt.Errorf("waivers: %w", err)
		}
	}
	var m SystemModel
	if err := inst.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("model has no name")
	}
	if err := validateWaivers(&m); err != nil {
		return nil, fmt.Errorf("model %q: %w", m.Name, err)
	}
	desired, err := decodeDesired(inst)
	if err != nil {
		return nil, fmt.Errorf("decode desired: %w", err)
	}
	if desired != nil {
		m.Desired = desired
	}
	if !inst.LookupPath(cue.ParsePath("populate")).Exists() {
		m.Populate.Absent = true
		if len(m.Desired) > 0 {
			return nil, fmt.Errorf("model %q has no populate arm, so it is checks-only and cannot declare desired", m.Name)
		}
	}
	if err := decodeTransforms(inst, &m); err != nil {
		return nil, fmt.Errorf("model %q: %w", m.Name, err)
	}
	if err := m.Populate.validateItemPlaceholders(); err != nil {
		return nil, fmt.Errorf("model %q: %w", m.Name, err)
	}
	if _, err := m.Populate.EmptyPatterns(); err != nil {
		return nil, fmt.Errorf("model %q: %w", m.Name, err)
	}
	if _, err := m.Populate.FailPatterns(); err != nil {
		return nil, fmt.Errorf("model %q: %w", m.Name, err)
	}
	if err := m.Populate.validateFallback(); err != nil {
		return nil, fmt.Errorf("model %q: %w", m.Name, err)
	}
	return &m, nil
}

// validateWaivers rejects a model whose waivers are incomplete, carry a bad
// date, or target a check the model does not have. Waivers only apply to
// checks, so a model without checks cannot declare any.
func validateWaivers(m *SystemModel) error {
	if len(m.Waivers) == 0 {
		return nil
	}
	if len(m.Checks) == 0 {
		return fmt.Errorf("waivers: the model declares no checks to waive")
	}
	expects := make(map[string]string, len(m.Checks))
	for _, c := range m.Checks {
		expects[c.Name] = c.Expect
	}
	return waiver.Validate(m.Waivers, expects)
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
		// Optional constraints are not observed fields. Including them would
		// invent expectations such as an unset depends_on list.
		fields, err := iter.Value().Fields(cue.Hidden(true))
		if err != nil {
			return nil, err
		}
		for fields.Next() {
			var val any
			if err := fields.Value().Decode(&val); err != nil {
				return nil, err
			}
			selector := fields.Selector()
			name := selector.String()
			if selector.IsString() {
				// String() is CUE syntax, including quotes for labels such as
				// "_schema". Desired records need the actual field name.
				name = selector.Unquoted()
			}
			rec[name] = val
		}
		out = append(out, rec)
	}
	return out, nil
}

// Observation declares the population a successful collection covers.
type Observation struct {
	Scope    string   `json:"scope,omitempty"`
	Complete bool     `json:"complete,omitempty"`
	Schemas  []string `json:"schemas,omitempty"`
}
