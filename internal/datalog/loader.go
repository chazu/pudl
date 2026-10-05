package datalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

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
		// A search path that is not a readable directory contributes no rules; it
		// is not an error. A workspace legitimately may not have a rules dir, and
		// a path that exists but is a file (ENOTDIR) is the same nothing from the
		// caller's point of view. Tolerating it here rather than at each caller is
		// what lets every caller share one unfiltered search order.
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			return nil, nil
		}
		return nil, err
	}

	var rules []Rule
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".cue") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		fileRules, err := parseRules(ctx, string(data), path)
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
//
// A top-level struct field with a "head" or a "body" is a rule. Other fields are
// ignored. A rule that cannot be read as written is returned with LoadErr set
// (see Rule) instead of being skipped; only a source that does not compile at
// all is an error.
func ParseRules(ctx *cue.Context, source string) ([]Rule, error) {
	return parseRules(ctx, source, "")
}

// parseRules is ParseRules with a filename for source positions.
func parseRules(ctx *cue.Context, source, filename string) ([]Rule, error) {
	var opts []cue.BuildOption
	if filename != "" {
		opts = append(opts, cue.Filename(filename))
	}
	v := ctx.CompileString(source, opts...)
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
		if !looksLikeRule(fieldVal) {
			continue
		}
		rule := extractRule(iter.Selector().Unquoted(), fieldVal)
		if rule.LoadErr == nil {
			rule.LoadErr = checkRule(rule)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// looksLikeRule reports whether a top-level field is meant to be a rule: a
// struct with a head or a body. Anything else (constants, helper values) is not
// a rule and is ignored.
func looksLikeRule(v cue.Value) bool {
	if v.IncompleteKind() != cue.StructKind {
		return false
	}
	return v.LookupPath(cue.ParsePath("head")).Exists() || v.LookupPath(cue.ParsePath("body")).Exists()
}

// extractRule converts a CUE value into a Rule. A rule that cannot be read is
// returned with LoadErr set and as much of its head as could be read.
func extractRule(fieldName string, v cue.Value) Rule {
	rule := Rule{Name: fieldName, Source: positionOf(v)}

	if nameVal := v.LookupPath(cue.ParsePath("name")); nameVal.Exists() {
		if s, err := nameVal.String(); err == nil {
			rule.Name = s
		}
	}

	fail := func(err error) Rule {
		rule.LoadErr = err
		return rule
	}

	headVal := v.LookupPath(cue.ParsePath("head"))
	if !headVal.Exists() {
		return fail(fmt.Errorf("missing head"))
	}
	if rel, err := headVal.LookupPath(cue.ParsePath("rel")).String(); err == nil {
		rule.Head.Rel = rel
	}
	head, err := extractAtom(headVal)
	if err != nil {
		return fail(fmt.Errorf("bad head: %w", err))
	}
	rule.Head = head

	bodyVal := v.LookupPath(cue.ParsePath("body"))
	if !bodyVal.Exists() {
		return fail(fmt.Errorf("missing body"))
	}
	bodyList, err := bodyVal.List()
	if err != nil {
		return fail(fmt.Errorf("body not a list: %w", err))
	}

	for i := 0; bodyList.Next(); i++ {
		atom, err := extractAtom(bodyList.Value())
		if err != nil {
			return fail(fmt.Errorf("bad body atom %d: %w", i, err))
		}
		rule.Body = append(rule.Body, atom)
	}

	return rule
}

// positionOf renders a CUE value's source position, or "" when unknown.
func positionOf(v cue.Value) string {
	pos := v.Pos()
	if !pos.IsValid() {
		return ""
	}
	return pos.String()
}

// extractAtom converts a CUE value into an Atom.
func extractAtom(v cue.Value) (Atom, error) {
	relVal := v.LookupPath(cue.ParsePath("rel"))
	argsVal := v.LookupPath(cue.ParsePath("args"))

	if !relVal.Exists() {
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
			// Unquoted: a CUE label written as "my-key" is the key my-key, not
			// a key containing quote characters.
			key := argsIter.Selector().Unquoted()
			if strings.Contains(key, `"`) {
				return Atom{}, fmt.Errorf("argument key %q must not contain a double quote", key)
			}
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
	case cue.IntKind, cue.FloatKind:
		raw, err := v.MarshalJSON()
		if err != nil {
			return Term{}, err
		}
		// Retain the literal until the compiler checks the query numeric domain.
		// Ignoring Int64/Float64 conversion errors used to clamp/round constants.
		return Val(json.Number(raw)), nil
	case cue.BoolKind:
		b, _ := v.Bool()
		return Val(b), nil
	default:
		return Term{}, fmt.Errorf("unsupported term kind: %v", v.Kind())
	}
}
