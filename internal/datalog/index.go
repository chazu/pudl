package datalog

import "fmt"

// index provides O(1) lookup of tuples by relation + arg key + arg value.
// Structure: relation -> argKey -> argValue -> []Tuple
type index struct {
	data map[string]map[string]map[string][]Tuple
}

func newIndex() *index {
	return &index{data: make(map[string]map[string]map[string][]Tuple)}
}

// add indexes a tuple on all of its arg keys.
func (idx *index) add(t Tuple) {
	rel := t.Relation
	if idx.data[rel] == nil {
		idx.data[rel] = make(map[string]map[string][]Tuple)
	}
	for key, val := range t.Args {
		valStr := valueKey(val)
		if idx.data[rel][key] == nil {
			idx.data[rel][key] = make(map[string][]Tuple)
		}
		idx.data[rel][key][valStr] = append(idx.data[rel][key][valStr], t)
	}
}

// lookup returns tuples matching a relation + arg key + arg value.
// Returns nil if no index exists for that combination.
func (idx *index) lookup(relation, argKey string, argValue interface{}) []Tuple {
	byKey := idx.data[relation]
	if byKey == nil {
		return nil
	}
	byVal := byKey[argKey]
	if byVal == nil {
		return nil
	}
	return byVal[valueKey(argValue)]
}

// all returns all indexed tuples for a relation.
func (idx *index) all(relation string) []Tuple {
	byKey := idx.data[relation]
	if byKey == nil {
		return nil
	}
	// Use the dedup set to avoid returning the same tuple multiple times
	// (it's indexed under multiple arg keys)
	seen := make(map[string]bool)
	var result []Tuple
	for _, byVal := range byKey {
		for _, tuples := range byVal {
			for _, t := range tuples {
				k := t.Key()
				if !seen[k] {
					seen[k] = true
					result = append(result, t)
				}
			}
		}
	}
	return result
}

// buildIndex creates an index from a flat map of tuples.
func buildIndex(tuples map[string]Tuple) *index {
	idx := newIndex()
	for _, t := range tuples {
		idx.add(t)
	}
	return idx
}

// valueKey converts a value to a string key for indexing.
func valueKey(v interface{}) string {
	return fmt.Sprintf("%v", v)
}
