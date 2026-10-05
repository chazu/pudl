package acute

import (
	"encoding/json"
	"sort"
	"time"
)

// PreviousInventory names a single eligible earlier snapshot or why none is usable.
type PreviousInventory struct {
	Status     string
	SnapshotID string
	ObservedAt *time.Time
	Records    []ObservedRecord
}

func attachPrevious(drift *ResourceDrift, desired map[string]any, identity IdentityResolver, history PreviousInventory) {
	previous := PreviousValue{Status: history.Status, SnapshotID: history.SnapshotID, ObservedAt: history.ObservedAt}
	var record map[string]any
	if history.Status == "available" {
		key, _, ok := RecordIdentity(desired, identity)
		if !ok {
			previous.Status = "incompatible"
		} else {
			var matches []ObservedRecord
			schema, _ := desired["_schema"].(string)
			var fields []string
			if identity != nil {
				fields = identity(schema)
			}
			declared := len(fields) > 0
			// RecordIdentity uses name/path/id when a resource-type tag has no
			// directly resolvable schema definition. Compare that actual contract
			// with the retained identity, rather than treating an empty declaration
			// as different from the same named fallback field.
			if !declared {
				fields = fallbackIdentityFields(desired)
			}
			fields = append([]string{}, fields...)
			sort.Strings(fields)
			incompatible := false
			for _, candidate := range history.Records {
				candidateSchema, _ := candidate.Data["_schema"].(string)
				if candidateSchema != schema {
					continue
				}
				recorded := append([]string{}, candidate.IdentityFields...)
				if len(recorded) == 0 && !declared {
					recorded = fallbackIdentityFields(candidate.Data)
				}
				sort.Strings(recorded)
				if !sameFields(fields, recorded) {
					incompatible = true
					continue
				}
				if k, _, ok := RecordIdentity(candidate.Data, identity); ok && k == key {
					matches = appendDistinct(matches, candidate)
				}
			}
			switch {
			case incompatible:
				previous.Status = "incompatible"
			case len(matches) == 0:
				previous.Status = "absent"
			case len(matches) > 1:
				previous.Status = "ambiguous"
			default:
				record = matches[0].Data
				projected := map[string]any{}
				for key := range desired {
					if value, ok := record[key]; ok {
						projected[key] = value
					}
				}
				previous.Value, _ = json.Marshal(projected)
			}
		}
	}
	drift.Previous = &previous
	for i := range drift.Fields {
		field := &drift.Fields[i]
		value := previous
		value.Value = nil
		if record != nil {
			v, ok := valueAt(record, field.PathComponents)
			if ok {
				value.Value, _ = json.Marshal(v)
			} else {
				value.Status = "absent"
			}
		}
		field.Previous = &value
	}
}
func sameFields(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func valueAt(record any, path []PathComponent) (any, bool) {
	value := record
	for _, part := range path {
		if part.Key != nil {
			object, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			value, ok = object[*part.Key]
			if !ok {
				return nil, false
			}
			continue
		}
		if part.Index != nil {
			list, ok := value.([]any)
			if !ok || *part.Index < 0 || *part.Index >= len(list) {
				return nil, false
			}
			value = list[*part.Index]
			continue
		}
		return nil, false
	}
	return value, true
}

func fallbackIdentityFields(record map[string]any) []string {
	for _, key := range []string{"name", "path", "id"} {
		if _, ok := record[key]; ok {
			return []string{key}
		}
	}
	return nil
}
