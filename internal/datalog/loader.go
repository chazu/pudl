package datalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// LoadRulesFromPaths loads #Rule definitions from CUE files in the given
// directories. Later paths take precedence (repo-scoped shadows global).
// Files must contain top-level fields that conform to the #Rule shape.
func LoadRulesFromPaths(paths ...string) ([]Rule, error) {
	ctx := cuecontext.New()
	seen := make(map[string]bool) // rule names, for shadowing

	var rules []Rule

	// Process in reverse order so later paths (repo-scoped) shadow earlier (global)
	for i := len(paths) - 1; i >= 0; i-- {
		dirRules, err := loadRulesFromDir(ctx, paths[i])
		if err != nil {
			return nil, fmt.Errorf("loading rules from %s: %w", paths[i], err)
		}
		for _, r := range dirRules {
			if r.Name != "" && seen[r.Name] {
				continue // shadowed by higher-priority source
			}
			if r.Name != "" {
				seen[r.Name] = true
			}
			rules = append(rules, r)
		}
	}

	return rules, nil
}

// loadRulesFromDir reads all .cue files in a directory and extracts rules.
func loadRulesFromDir(ctx *cue.Context, dir string) ([]Rule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // missing directory is fine
		}
		return nil, err
	}

	var rules []Rule
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cue") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		fileRules, err := ParseRules(ctx, string(data))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		rules = append(rules, fileRules...)
	}

	return rules, nil
}

// ParseRulesFromSource is a convenience wrapper that creates its own CUE context.
func ParseRulesFromSource(source string) ([]Rule, error) {
	ctx := cuecontext.New()
	return ParseRules(ctx, source)
}

// ParseRules extracts Rule values from a CUE source string.
// Each top-level field with a "head" and "body" is treated as a rule.
func ParseRules(ctx *cue.Context, source string) ([]Rule, error) {
	v := ctx.CompileString(source)
	if v.Err() != nil {
		return nil, fmt.Errorf("CUE compile: %w", v.Err())
	}

	var rules []Rule

	// Walk top-level fields
	iter, err := v.Fields(cue.Definitions(false), cue.Optional(false))
	if err != nil {
		return nil, fmt.Errorf("iterating fields: %w", err)
	}

	for iter.Next() {
		fieldVal := iter.Value()
		rule, err := extractRule(iter.Selector().String(), fieldVal)
		if err != nil {
			continue // skip non-rule fields
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// extractRule converts a CUE value into a Rule.
func extractRule(fieldName string, v cue.Value) (Rule, error) {
	headVal := v.LookupPath(cue.ParsePath("head"))
	bodyVal := v.LookupPath(cue.ParsePath("body"))

	if headVal.Err() != nil || bodyVal.Err() != nil {
		return Rule{}, fmt.Errorf("not a rule: missing head or body")
	}

	// Extract name (optional)
	name := fieldName
	if nameVal := v.LookupPath(cue.ParsePath("name")); nameVal.Err() == nil {
		if s, err := nameVal.String(); err == nil {
			name = s
		}
	}

	head, err := extractAtom(headVal)
	if err != nil {
		return Rule{}, fmt.Errorf("bad head: %w", err)
	}

	bodyList, err := bodyVal.List()
	if err != nil {
		return Rule{}, fmt.Errorf("body not a list: %w", err)
	}

	var body []Atom
	for bodyList.Next() {
		atom, err := extractAtom(bodyList.Value())
		if err != nil {
			return Rule{}, fmt.Errorf("bad body atom: %w", err)
		}
		body = append(body, atom)
	}

	if len(body) == 0 {
		return Rule{}, fmt.Errorf("empty body")
	}

	return Rule{Name: name, Head: head, Body: body}, nil
}

// extractAtom converts a CUE value into an Atom.
func extractAtom(v cue.Value) (Atom, error) {
	relVal := v.LookupPath(cue.ParsePath("rel"))
	argsVal := v.LookupPath(cue.ParsePath("args"))

	if relVal.Err() != nil {
		return Atom{}, fmt.Errorf("missing rel")
	}

	rel, err := relVal.String()
	if err != nil {
		return Atom{}, fmt.Errorf("rel not a string: %w", err)
	}

	args := make(map[string]Term)
	if argsVal.Err() == nil {
		argsIter, err := argsVal.Fields()
		if err != nil {
			return Atom{}, fmt.Errorf("args not a struct: %w", err)
		}
		for argsIter.Next() {
			key := argsIter.Selector().String()
			term, err := extractTerm(argsIter.Value())
			if err != nil {
				return Atom{}, fmt.Errorf("bad term %s: %w", key, err)
			}
			args[key] = term
		}
	}

	return Atom{Rel: rel, Args: args}, nil
}

// extractTerm converts a CUE value into a Term.
func extractTerm(v cue.Value) (Term, error) {
	switch v.Kind() {
	case cue.StringKind:
		s, _ := v.String()
		return ParseTerm(s), nil
	case cue.IntKind:
		n, _ := v.Int64()
		return Val(n), nil
	case cue.FloatKind:
		f, _ := v.Float64()
		return Val(f), nil
	case cue.BoolKind:
		b, _ := v.Bool()
		return Val(b), nil
	default:
		return Term{}, fmt.Errorf("unsupported term kind: %v", v.Kind())
	}
}
