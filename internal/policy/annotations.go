package policy

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/literal"
)

// checkAnnotations makes sure every `@pudl` attribute written in a chain
// member's source reached the tree. The tree is read from the evaluated
// definition, which cannot show a field declared only inside a comprehension
// that the bare policy cannot decide (its condition reads record data), nor a
// field under a pattern constraint. Such an annotation would be dropped, so
// the policy fails to load instead.
func checkAnnotations(p *Policy, root *node, rules map[string]*Rule) error {
	for _, m := range p.members {
		c := annotationCollector{ctx: m.value.Context()}
		c.expr(m.value.Syntax(cue.Raw(), cue.Attributes(true), cue.Optional(true), cue.Definitions(true)), nil, true)
		for _, a := range c.found {
			if err := a.check(root, rules); err != nil {
				if m.name != p.Name {
					return fmt.Errorf("in %s: %w", m.name, err)
				}
				return err
			}
		}
	}
	return nil
}

// sourceAnnotation is a `@pudl` attribute as written in a member's source.
type sourceAnnotation struct {
	text string
	segs []string
	// static: every label from the member's root down to the field is a
	// literal, so segs is the field's tree path.
	static bool
	rule   string
	absent bool
	err    error
}

func (a sourceAnnotation) check(root *node, rules map[string]*Rule) error {
	if a.err != nil {
		return fmt.Errorf("%s: invalid @pudl attribute: %w", a.where(), a.err)
	}
	applied := true
	if a.static {
		n := root.at(a.segs)
		applied = n != nil &&
			(a.rule == "" || (n.rule != nil && n.rule.Name == a.rule)) &&
			(!a.absent || n.absentViolation)
	} else if a.rule != "" {
		_, applied = rules[a.rule]
	}
	if applied {
		return nil
	}
	return fmt.Errorf("%s: %s cannot be applied: the field is declared only inside a comprehension "+
		"whose condition depends on the record, or under a dynamic label; declare the field and its "+
		"annotation outside the comprehension (for example `field?: _ %s`)", a.where(), a.text, a.text)
}

func (a sourceAnnotation) where() string {
	if a.static {
		return pathString(a.segs)
	}
	return "a field with a dynamic label"
}

// annotationCollector walks CUE syntax and records `@pudl` field attributes
// with the tree path of their field. Comprehension bodies and embeddings
// declare fields of the enclosing struct; list elements are [*].
type annotationCollector struct {
	ctx   *cue.Context
	found []sourceAnnotation
}

func (c *annotationCollector) expr(n ast.Node, segs []string, static bool) {
	switch x := n.(type) {
	case *ast.StructLit:
		c.decls(x.Elts, segs, static)
	case *ast.File:
		c.decls(x.Decls, segs, static)
	case *ast.ListLit:
		el := append(append([]string(nil), segs...), wildcard)
		for _, e := range x.Elts {
			if ell, ok := e.(*ast.Ellipsis); ok {
				if ell.Type != nil {
					c.expr(ell.Type, el, static)
				}
				continue
			}
			c.expr(e, el, static)
		}
	case *ast.BinaryExpr:
		c.expr(x.X, segs, static)
		c.expr(x.Y, segs, static)
	case *ast.ParenExpr:
		c.expr(x.X, segs, static)
	}
}

func (c *annotationCollector) decls(decls []ast.Decl, segs []string, static bool) {
	for _, d := range decls {
		switch x := d.(type) {
		case *ast.Field:
			key, literalLabel, regular := fieldLabel(x.Label)
			if !regular {
				continue // hidden fields and definitions are never record data
			}
			cs := append(append([]string(nil), segs...), key)
			cstatic := static && literalLabel
			for _, attr := range x.Attrs {
				if name, _ := attr.Split(); name == "pudl" {
					c.found = append(c.found, c.parse(attr, cs, cstatic))
				}
			}
			c.expr(x.Value, cs, cstatic)
		case *ast.Comprehension:
			c.expr(x.Value, segs, static)
		case *ast.EmbedDecl:
			c.expr(x.Expr, segs, static)
		}
	}
}

// parse reads an attribute's rule and absent keys the way CUE does, by
// attaching it to a throwaway field.
func (c *annotationCollector) parse(attr *ast.Attribute, segs []string, static bool) sourceAnnotation {
	a := sourceAnnotation{text: attr.Text, segs: segs, static: static}
	pa := c.ctx.CompileString("x: _ " + attr.Text).LookupPath(cue.ParsePath("x")).Attribute("pudl")
	if a.err = pa.Err(); a.err != nil {
		return a
	}
	if a.rule, _, a.err = pa.Lookup(0, "rule"); a.err != nil {
		return a
	}
	var absent string
	absent, _, a.err = pa.Lookup(0, "absent")
	a.absent = absent != ""
	return a
}

// fieldLabel returns a field label's key, whether it is a literal, and whether
// it is a regular field (not hidden, not a definition).
func fieldLabel(l ast.Label) (key string, literalLabel, regular bool) {
	switch x := l.(type) {
	case *ast.Ident:
		if strings.HasPrefix(x.Name, "_") || strings.HasPrefix(x.Name, "#") {
			return "", false, false
		}
		return x.Name, true, true
	case *ast.BasicLit:
		s, err := literal.Unquote(x.Value)
		if err != nil {
			return "", false, true
		}
		return s, true, true
	case *ast.Alias:
		if inner, ok := x.Expr.(ast.Label); ok {
			return fieldLabel(inner)
		}
	}
	return "", false, true
}
