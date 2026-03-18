package executor

import (
	"fmt"
)

// Effect describes a side effect that a method intends to perform.
// Methods can return effects in their output under the "pudl/effects" key,
// allowing the runtime to audit, dry-run, or batch-execute them.
type Effect struct {
	Kind        string                 // "create", "delete", "update", "http", "exec"
	Description string                 // human-readable summary
	Params      map[string]interface{} // effect-specific parameters
}

// EffectOutcome records the result of processing an effect.
type EffectOutcome struct {
	Effect Effect
	Status string // "executed", "skipped", "failed"
	Output interface{}
	Error  string
}

// ParseEffects checks if a method output contains a "pudl/effects" key and
// extracts the effect list. Returns the parsed effects and true if effects
// were found, or nil and false if the output contains no effects.
func ParseEffects(output interface{}) ([]Effect, bool) {
	m, ok := toStringKeyMap(output)
	if !ok {
		return nil, false
	}

	raw, exists := m["pudl/effects"]
	if !exists {
		return nil, false
	}

	return parseEffectList(raw)
}

// parseEffectList converts a raw effect list value into typed Effects.
func parseEffectList(raw interface{}) ([]Effect, bool) {
	// Handle []interface{} (most common from Glojure)
	list, ok := raw.([]interface{})
	if !ok {
		// Try Glojure sequence via toSlice
		list = toSlice(raw)
		if list == nil {
			return nil, false
		}
	}

	var effects []Effect
	for _, item := range list {
		e, err := parseOneEffect(item)
		if err != nil {
			continue // skip malformed effects
		}
		effects = append(effects, e)
	}

	if len(effects) == 0 {
		return nil, false
	}

	return effects, true
}

// parseOneEffect converts a single raw value into an Effect.
func parseOneEffect(raw interface{}) (Effect, error) {
	m, ok := toStringKeyMap(raw)
	if !ok {
		return Effect{}, fmt.Errorf("effect must be a map, got %T", raw)
	}

	e := Effect{
		Params: make(map[string]interface{}),
	}

	if kind, ok := m["kind"]; ok {
		if s, ok := kind.(string); ok {
			e.Kind = s
		}
	}
	if e.Kind == "" {
		return Effect{}, fmt.Errorf("effect missing required 'kind' field")
	}

	if desc, ok := m["description"]; ok {
		if s, ok := desc.(string); ok {
			e.Description = s
		}
	}

	if params, ok := m["params"]; ok {
		if pm, ok := toStringKeyMap(params); ok {
			e.Params = pm
		}
	}

	return e, nil
}

// toSlice attempts to convert a Glojure sequence into a Go slice.
func toSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}

	// Try iterating via Glojure Seq protocol
	type seqable interface {
		Seq() interface{}
	}
	type seq interface {
		First() interface{}
		Next() interface{}
	}

	if sa, ok := v.(seqable); ok {
		var result []interface{}
		s := sa.Seq()
		for s != nil {
			if sq, ok := s.(seq); ok {
				result = append(result, sq.First())
				s = sq.Next()
			} else {
				break
			}
		}
		return result
	}

	return nil
}

// FormatEffect returns a human-readable string for an effect.
func FormatEffect(e Effect) string {
	s := fmt.Sprintf("[%s] %s", e.Kind, e.Description)
	if len(e.Params) > 0 {
		s += fmt.Sprintf(" params=%v", e.Params)
	}
	return s
}

// FormatEffectOutcome returns a human-readable string for an effect outcome.
func FormatEffectOutcome(eo EffectOutcome) string {
	s := fmt.Sprintf("[%s] %s — %s", eo.Effect.Kind, eo.Effect.Description, eo.Status)
	if eo.Error != "" {
		s += fmt.Sprintf(" error: %s", eo.Error)
	}
	return s
}
