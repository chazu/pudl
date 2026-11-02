package policy

import (
	"fmt"
	"slices"

	"cuelang.org/go/cue"
)

// wildcard is the tree segment for "every element of a list".
const wildcard = "[*]"

// maxDepth bounds the walk of recursive definitions.
const maxDepth = 32

// node is one field of a policy's effective shape, merged across every schema
// in its chain. The tree carries what CUE errors do not: which fields are
// required, which rule annotates them, who required them.
type node struct {
	children map[string]*node
	order    []string
	// required: some schema in the chain declares the field non-optional.
	required bool
	// absentViolation: `@pudl(absent="violation")` on this field.
	absentViolation bool
	// rule: `@pudl(rule=...)` on this field.
	rule *Rule
	// requiredBy: the most-base schema in the chain that requires the field.
	requiredBy string
	// fromPolicy: a policy (not the type) declares the field.
	fromPolicy bool
}

func newNode() *node { return &node{children: map[string]*node{}} }

func (n *node) child(seg string) *node {
	c, ok := n.children[seg]
	if !ok {
		c = newNode()
		n.children[seg] = c
		n.order = append(n.order, seg)
	}
	return c
}

// treeBuilder merges chain members into one tree, most-base member first so
// requiredBy and rule DeclaredBy name the earliest declarer.
type treeBuilder struct {
	root  *node
	rules map[string]*Rule
}

func newTreeBuilder() *treeBuilder {
	return &treeBuilder{root: newNode(), rules: map[string]*Rule{}}
}

// merge adds the regular (non-hidden, non-definition) fields of v under n.
func (b *treeBuilder) merge(n *node, v cue.Value, segs []string, member string, isPolicy bool) error {
	if len(segs) > maxDepth {
		return nil
	}
	kind := v.IncompleteKind()
	if kind&cue.StructKind != 0 {
		iter, err := v.Fields(cue.Optional(true))
		if err == nil {
			for iter.Next() {
				key := iter.Selector().Unquoted()
				if err := b.field(n, key, iter.Value(), !iter.IsOptional(), append(segs, key), member, isPolicy); err != nil {
					return err
				}
			}
		}
	}
	if kind&cue.ListKind != 0 {
		if el := v.LookupPath(cue.MakePath(cue.AnyIndex)); el.Exists() {
			return b.field(n, wildcard, el, false, append(segs, wildcard), member, isPolicy)
		}
	}
	return nil
}

func (b *treeBuilder) field(parent *node, seg string, v cue.Value, required bool, segs []string, member string, isPolicy bool) error {
	c := parent.child(seg)
	if required && c.requiredBy == "" {
		c.requiredBy = member
	}
	c.required = c.required || required
	c.fromPolicy = c.fromPolicy || isPolicy
	if err := b.annotate(c, v, segs, member); err != nil {
		return err
	}
	return b.merge(c, v, append([]string(nil), segs...), member, isPolicy)
}

// annotate reads `@pudl(...)` field attributes. Allowed keys: rule, severity,
// doc and absent. severity and doc need a rule; absent must be "violation".
func (b *treeBuilder) annotate(n *node, v cue.Value, segs []string, member string) error {
	path := pathString(segs)
	for _, attr := range v.Attributes(cue.FieldAttr) {
		if attr.Name() != "pudl" {
			continue
		}
		if err := attr.Err(); err != nil {
			return fmt.Errorf("%s: invalid @pudl attribute: %w", path, err)
		}
		var rule Rule
		for i := 0; i < attr.NumArgs(); i++ {
			key, val := attr.Arg(i)
			switch key {
			case "rule":
				rule.Name = val
			case "severity":
				rule.Severity = Severity(val)
			case "doc":
				rule.Doc = val
			case "absent":
				if val != "violation" {
					return fmt.Errorf("%s: @pudl(absent=%q): the only value is \"violation\"", path, val)
				}
				n.absentViolation = true
			default:
				return fmt.Errorf("%s: unknown @pudl key %q (allowed: rule, severity, doc, absent)", path, key)
			}
		}
		if rule.Name == "" {
			if rule.Severity != "" || rule.Doc != "" {
				return fmt.Errorf("%s: @pudl severity and doc need a rule name", path)
			}
			continue
		}
		if rule.Severity == "" {
			rule.Severity = DefaultSeverity
		}
		if severityRank[rule.Severity] == 0 {
			return fmt.Errorf("%s: rule %q: invalid severity %q (want low, medium, high or critical)", path, rule.Name, rule.Severity)
		}
		if err := b.declareRule(n, rule, path, member); err != nil {
			return err
		}
	}
	return nil
}

func (b *treeBuilder) declareRule(n *node, rule Rule, path, member string) error {
	if n.rule != nil && n.rule.Name != rule.Name {
		return fmt.Errorf("%s: annotated with both rule %q and rule %q", path, n.rule.Name, rule.Name)
	}
	known, ok := b.rules[rule.Name]
	if !ok {
		rule.DeclaredBy = member
		known = &rule
		b.rules[rule.Name] = known
	} else if known.Severity != rule.Severity || known.Doc != rule.Doc {
		return fmt.Errorf("%s: rule %q redeclared with a different severity or doc", path, rule.Name)
	}
	if !slices.Contains(known.Paths, path) {
		known.Paths = append(known.Paths, path)
	}
	n.rule = known
	return nil
}

// lookup walks the tree along segs and returns the deepest node reached and
// the nearest rule at or above it.
func (n *node) lookup(segs []string) (*node, *Rule) {
	cur, rule := n, n.rule
	for _, s := range segs {
		next, ok := cur.children[s]
		if !ok {
			break
		}
		cur = next
		if cur.rule != nil {
			rule = cur.rule
		}
	}
	return cur, rule
}
