package systemmodel

import (
	"fmt"
	"strings"
)

// ItemPlaceholder is replaced by each enumerated input in a run's argv and in
// the string values of its set.
const ItemPlaceholder = "{{item}}"

// Enumeration is #Enumeration: a command whose JSON output lists the inputs
// a populate's runs are repeated for.
type Enumeration struct {
	Argv []string `json:"argv"`
	// Select is the field path of the input in each listed object; empty when
	// the command lists strings.
	Select string `json:"select,omitempty"`
	Dir    string `json:"dir,omitempty"`
}

// validateItemPlaceholders requires every run to use the item placeholder
// when the populate enumerates its inputs, and no run to use it otherwise: a
// run without it would repeat identically per input, and a placeholder
// without `each` would reach the command literally.
func (p Populate) validateItemPlaceholders() error {
	for i, run := range p.Runs {
		uses := run.usesItem()
		switch {
		case p.Each != nil && !uses:
			return fmt.Errorf("populate run %d does not use %s, so `each` would repeat it identically", i, ItemPlaceholder)
		case p.Each == nil && uses:
			return fmt.Errorf("populate run %d uses %s but the populate declares no `each`", i, ItemPlaceholder)
		}
	}
	return nil
}

func (r CommandRun) usesItem() bool {
	for _, arg := range r.Argv {
		if strings.Contains(arg, ItemPlaceholder) {
			return true
		}
	}
	return setUsesItem(r.Set)
}

func setUsesItem(v any) bool {
	switch v := v.(type) {
	case string:
		return strings.Contains(v, ItemPlaceholder)
	case map[string]any:
		for _, child := range v {
			if setUsesItem(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if setUsesItem(child) {
				return true
			}
		}
	}
	return false
}

// ForItem returns the run for one enumerated input: the placeholder replaced
// in argv and in set's string values. The receiver is not modified.
func (r CommandRun) ForItem(item string) CommandRun {
	out := CommandRun{Dir: r.Dir, Argv: make([]string, len(r.Argv)), Transform: r.Transform}
	for i, arg := range r.Argv {
		out.Argv[i] = strings.ReplaceAll(arg, ItemPlaceholder, item)
	}
	if r.Set != nil {
		out.Set, _ = substituteItem(r.Set, item).(map[string]any)
	}
	return out
}

func substituteItem(v any, item string) any {
	switch v := v.(type) {
	case string:
		return strings.ReplaceAll(v, ItemPlaceholder, item)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = substituteItem(child, item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = substituteItem(child, item)
		}
		return out
	}
	return v
}
