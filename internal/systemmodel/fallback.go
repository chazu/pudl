package systemmodel

import (
	"fmt"
	"regexp"
)

// Fallback is populate.fallback: another way to observe the same resources,
// used instead of the populate's own runs when they cannot answer. It is
// used when a run (or the `each` command) fails with stderr matching one of
// When, or when `each` lists no inputs. The fallback's records are routed
// like the populate's (its schema); it has its own runs, each, timeout,
// fail_fast, empty_when and fail_when, and no fallback of its own. A Partial fallback
// sees only some of the populate's resources, so an observation it answers is
// incomplete.
type Fallback struct {
	When    []string `json:"when"`
	Partial bool     `json:"partial,omitempty"`
	Observe Populate `json:"observe"`
}

// Patterns compiles fallback.when (Go RE2 syntax).
func (f *Fallback) Patterns() ([]*regexp.Regexp, error) {
	if len(f.When) == 0 {
		return nil, fmt.Errorf("populate.fallback.when needs at least one pattern")
	}
	out := make([]*regexp.Regexp, 0, len(f.When))
	for i, pattern := range f.When {
		if pattern == "" {
			return nil, fmt.Errorf("populate.fallback.when[%d] is empty, so it would match every failure", i)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("populate.fallback.when[%d] %q is not a valid regular expression: %w", i, pattern, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// validateFallback checks what CUE cannot: the patterns compile, the
// fallback's placeholders match its own each, and it routes nothing itself.
func (p Populate) validateFallback() error {
	f := p.Fallback
	if f == nil {
		return nil
	}
	if _, err := f.Patterns(); err != nil {
		return err
	}
	o := f.Observe
	switch {
	case o.Fallback != nil:
		return fmt.Errorf("populate.fallback.observe cannot declare its own fallback")
	case o.Schema != "" || o.AllowSchemaFallback:
		return fmt.Errorf("populate.fallback.observe cannot set schema or allow_schema_fallback: its records are routed like the populate's")
	}
	if err := o.validateItemPlaceholders(); err != nil {
		return fmt.Errorf("populate.fallback.observe: %w", err)
	}
	if _, err := o.EmptyPatterns(); err != nil {
		return fmt.Errorf("populate.fallback.observe: %w", err)
	}
	if _, err := o.FailPatterns(); err != nil {
		return fmt.Errorf("populate.fallback.observe: %w", err)
	}
	return nil
}
