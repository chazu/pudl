package policy

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"

	"github.com/chazu/pudl/internal/schemaname"
)

// closednessError is the message CUE reports when a closed struct (any
// definition) meets a field it does not declare. Policy evaluation is open:
// extra record fields, and fields a policy adds beyond its base, never fail.
const closednessError = "field not allowed"

// Evaluate checks record against policy p. typeName is the record's assigned
// type schema; p must apply to it (see Applicable).
//
// Conflicts come from unifying the record with each schema of the policy's
// chain and keeping errors at paths the record has. Absences come from
// walking the policy's required fields against the record.
func (s *Set) Evaluate(p *Policy, typeName string, record map[string]any) (*Result, error) {
	typeName = schemaname.Normalize(typeName)
	typeMeta, ok := s.src.GetSchemaMetadata(typeName)
	if !ok {
		return nil, fmt.Errorf("type schema %s is not loaded", typeName)
	}
	if !s.applies(p, typeName) {
		return nil, fmt.Errorf("policy %s does not apply to type %s", p.Name, typeName)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode record: %w", err)
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("decode record: %w", err)
	}

	f := &findings{seen: map[findingKey]bool{}}
	if err := s.conflicts(p, raw, data, f); err != nil {
		return nil, err
	}
	a := absenceWalk{set: s, policy: p, typeName: typeName, f: f}
	a.walk(p.tree, data, nil, nil, false)

	res := &Result{
		Policy:      p.Name,
		Type:        typeName,
		Digest:      p.Digest,
		Violations:  f.violations,
		Unknown:     f.unknown,
		Provisional: !typeMeta.IsValidated(),
	}
	SortViolations(res.Violations)
	SortViolations(res.Unknown)
	switch {
	case len(res.Violations) > 0:
		res.Status = StatusNoncompliant
	case len(res.Unknown) > 0:
		res.Status = StatusUnknown
	default:
		res.Status = StatusCompliant
	}
	return res, nil
}

// applies reports whether p's chain reaches typeName or one of its ancestors.
// A policy is never a record's type, so nothing applies to one.
func (s *Set) applies(p *Policy, typeName string) bool {
	if meta, ok := s.src.GetSchemaMetadata(typeName); !ok || meta.SchemaType == SchemaTypePolicy {
		return false
	}
	for _, t := range s.src.GetInheritanceGraph().GetCascadeChain(typeName) {
		if t == p.Type {
			return true
		}
	}
	return false
}

// findings collects violations and unknowns without duplicates.
type findings struct {
	violations []Violation
	unknown    []Violation
	seen       map[findingKey]bool
}

// findingKey identifies a finding regardless of which schema reported it.
type findingKey struct {
	rule, path string
	kind       Kind
	violation  bool
}

func (f *findings) add(v Violation, violation bool) {
	key := findingKey{v.Rule, v.Path, v.Kind, violation}
	if f.seen[key] {
		return
	}
	f.seen[key] = true
	if violation {
		f.violations = append(f.violations, v)
	} else {
		f.unknown = append(f.unknown, v)
	}
}

// violationAt builds a violation for canonical segments, attributing it to the
// nearest rule at or above them. An unannotated constraint is named by its
// path and attributed to fallback, the schema that imposed it.
func (p *Policy) violationAt(segs []string, kind Kind, fallback string) Violation {
	_, rule := p.tree.lookup(segs)
	path := pathString(segs)
	if rule != nil {
		return Violation{Rule: rule.Name, Severity: rule.Severity, Path: path, Kind: kind, DeclaredBy: rule.DeclaredBy}
	}
	if fallback == "" {
		fallback = p.Name
	}
	return Violation{Rule: path, Severity: DefaultSeverity, Path: path, Kind: kind, DeclaredBy: fallback}
}

// conflicts unifies the record with each schema of the chain separately. For a
// concrete record this finds the same data conflicts as unifying the whole
// chain at once, and it works when the chain spans schema paths (and so CUE
// runtimes). Validation is not concrete: a constraint the record leaves
// incomplete is an absence, found by the walk, not a conflict. Members are
// visited most-base first, so an unannotated conflict is attributed to the
// first schema in the chain that rejects the value.
func (s *Set) conflicts(p *Policy, raw []byte, data any, f *findings) error {
	compiled := map[*cue.Context]cue.Value{}
	for i := len(p.members) - 1; i >= 0; i-- {
		m := p.members[i]
		ctx := m.value.Context()
		dv, ok := compiled[ctx]
		if !ok {
			dv = ctx.CompileBytes(raw)
			if dv.Err() != nil {
				return fmt.Errorf("compile record: %w", dv.Err())
			}
			compiled[ctx] = dv
		}
		root := len(m.value.Path().Selectors())
		for _, e := range cueerrors.Errors(m.value.Unify(dv).Validate()) {
			if format, _ := e.Msg(); format == closednessError {
				continue
			}
			labels := e.Path()
			if len(labels) < root {
				continue
			}
			segs, present := recordPath(data, labels[root:])
			if !present {
				continue
			}
			f.add(p.violationAt(segs, KindConflict, m.name), true)
		}
	}
	return nil
}

// absenceWalk finds required policy fields the record lacks.
type absenceWalk struct {
	set      *Set
	policy   *Policy
	typeName string
	f        *findings
}

// walk visits the children of n against the record value at segs. rule is the
// nearest rule above; absentV is true when an ancestor opts into
// `absent="violation"`.
func (a *absenceWalk) walk(n *node, value any, segs []string, rule *Rule, absentV bool) {
	for _, seg := range n.order {
		c := n.children[seg]
		cs := append(append([]string(nil), segs...), seg)
		crule := rule
		if c.rule != nil {
			crule = c.rule
		}
		cabsent := absentV || c.absentViolation
		if seg == wildcard {
			if list, ok := value.([]any); ok {
				for _, el := range list {
					a.walk(c, el, cs, crule, cabsent)
				}
			}
			continue
		}
		obj, ok := value.(map[string]any)
		if !ok {
			continue // a non-object where the policy expects fields is a conflict
		}
		if v, present := obj[seg]; present {
			a.walk(c, v, cs, crule, cabsent)
			continue
		}
		if c.required {
			a.missing(c, cs, crule, cabsent)
		}
	}
}

// missing reports a required field the record lacks. When neither it nor an
// ancestor carries a rule, each annotated required field beneath it is
// reported under its own rule, so waivers and counts stay per rule; the field
// itself is reported too when a required constraint beneath it has no rule, or
// when nothing beneath it is annotated.
func (a *absenceWalk) missing(n *node, segs []string, rule *Rule, absentV bool) {
	collected := a.requiredInValidatedType(segs)
	if rule != nil {
		a.f.add(a.policy.violationAt(segs, KindAbsent, n.requiredBy), absentV || collected)
		return
	}
	uncovered, reported := false, 0
	var units func(n *node, segs []string, absentV bool)
	units = func(n *node, segs []string, absentV bool) {
		for _, seg := range n.order {
			c := n.children[seg]
			if seg == wildcard || !c.required {
				continue
			}
			cs := append(append([]string(nil), segs...), seg)
			cabsent := absentV || c.absentViolation
			switch {
			case c.rule != nil:
				a.f.add(a.policy.violationAt(cs, KindAbsent, c.requiredBy), cabsent || collected)
				reported++
			case len(c.order) == 0:
				uncovered = true
			default:
				units(c, cs, cabsent)
			}
		}
	}
	units(n, segs, absentV)
	if uncovered || reported == 0 {
		a.f.add(a.policy.violationAt(segs, KindAbsent, n.requiredBy), absentV || collected)
	}
}

// requiredInValidatedType reports whether the field at segs is required in a
// validated type schema of the record's type chain: such a field is always
// collected, so its absence is a violation.
func (a *absenceWalk) requiredInValidatedType(segs []string) bool {
	src := a.set.src
	for _, t := range src.GetInheritanceGraph().GetCascadeChain(a.typeName) {
		meta, ok := src.GetSchemaMetadata(t)
		if !ok || meta.SchemaType == SchemaTypePolicy || !meta.IsValidated() {
			continue
		}
		if v, ok := src.GetSchemaValue(t); ok && requiredAt(v, segs) {
			return true
		}
	}
	return false
}

// requiredAt reports whether v declares a non-optional field at segs.
func requiredAt(v cue.Value, segs []string) bool {
	cur := v
	for i, seg := range segs {
		if seg == wildcard {
			cur = cur.LookupPath(cue.MakePath(cue.AnyIndex))
			if !cur.Exists() {
				return false
			}
			continue
		}
		next, optional, found := fieldOf(cur, seg)
		if !found {
			return false
		}
		if i == len(segs)-1 {
			return !optional
		}
		cur = next
	}
	return false
}

func fieldOf(v cue.Value, key string) (cue.Value, bool, bool) {
	iter, err := v.Fields(cue.Optional(true))
	if err != nil {
		return cue.Value{}, false, false
	}
	for iter.Next() {
		if iter.Selector().Unquoted() == key {
			return iter.Value(), iter.IsOptional(), true
		}
	}
	return cue.Value{}, false, false
}
