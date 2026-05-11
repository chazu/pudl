package pithdriver

import (
	"encoding/json"
	"fmt"

	"github.com/chazu/pith"
)

// mapToStruct converts a map[string]any to a typed struct via JSON round-trip.
// Target struct must have json tags.
func mapToStruct[T any](m map[string]any) (T, error) {
	var out T
	b, err := json.Marshal(m)
	if err != nil {
		return out, fmt.Errorf("marshal: %w", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("unmarshal: %w", err)
	}
	return out, nil
}

// structToMap converts a typed struct to map[string]any via JSON round-trip.
func structToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return m, nil
}

// structsToMaps converts a slice of structs to []any of map[string]any.
func structsToMaps[T any](items []T) ([]any, error) {
	result := make([]any, len(items))
	for i, item := range items {
		m, err := structToMap(item)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		result[i] = m
	}
	return result, nil
}

// popMap pops TOS and asserts it is a map[string]any.
func popMap(vm *pith.VM) (map[string]any, error) {
	v, err := vm.Pop()
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", v)
	}
	return m, nil
}

// popString pops TOS and asserts it is a string.
func popString(vm *pith.VM) (string, error) {
	v, err := vm.Pop()
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", v)
	}
	return s, nil
}
