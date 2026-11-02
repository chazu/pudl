package policy

import (
	"fmt"
	"sort"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/redact"
	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// SchemaTypePolicy is the `_pudl.schema_type` of a policy schema.
const SchemaTypePolicy = validator.SchemaTypePolicy

// Source is the loaded schema set the engine reads. *inference.SchemaInferrer
// satisfies it.
type Source interface {
	redact.Source
	GetSchemaValue(name string) (cue.Value, bool)
}

// Policy is a loaded, checked policy.
type Policy struct {
	Name string `json:"name"`
	// Type is the type schema the policy's chain ends at.
	Type string `json:"type"`
	// Extends is the policy this one extends, or "" when it extends a type.
	Extends string `json:"extends,omitempty"`
	// Chain is the base_schema chain from the policy to the root type.
	Chain []string `json:"chain"`
	// Digest is a stable digest of the policy's effective CUE.
	Digest string `json:"digest"`
	// Rules are the named rules, inherited ones included, sorted by name.
	Rules []Rule `json:"rules"`

	members []member
	tree    *node
}

// member is one schema of a policy's chain with its loaded CUE value.
type member struct {
	name     string
	value    cue.Value
	isPolicy bool
}

// LoadError reports a policy that failed to load. It is excluded from the Set.
type LoadError struct {
	Policy string
	Err    error
}

func (e LoadError) Error() string { return fmt.Sprintf("policy %s: %v", e.Policy, e.Err) }

// Set is every loadable policy of a schema source.
type Set struct {
	src      Source
	policies map[string]*Policy
}

// Load finds every policy schema in src and checks it: its chain must end at a
// type, its @pudl attributes must be valid, and it must not constrain a path
// declared in sensitive_fields. Policies that fail are reported and left out.
func Load(src Source) (*Set, []LoadError) {
	set := &Set{src: src, policies: map[string]*Policy{}}
	sensitive := redact.NewRegistry(src)
	var errs []LoadError
	names := src.GetAvailableSchemas()
	sort.Strings(names)
	for _, name := range names {
		meta, ok := src.GetSchemaMetadata(name)
		if !ok || meta.SchemaType != SchemaTypePolicy {
			continue
		}
		p, err := loadPolicy(src, sensitive, name, meta)
		if err != nil {
			errs = append(errs, LoadError{Policy: name, Err: err})
			continue
		}
		set.policies[name] = p
	}
	return set, errs
}

func loadPolicy(src Source, sensitive *redact.Registry, name string, meta validator.SchemaMetadata) (*Policy, error) {
	if meta.BaseSchema == "" {
		return nil, fmt.Errorf("a policy must declare _pudl.base_schema")
	}
	p := &Policy{Name: name, Chain: src.GetInheritanceGraph().GetCascadeChain(name)}
	for _, m := range p.Chain {
		mmeta, ok := src.GetSchemaMetadata(m)
		value, found := src.GetSchemaValue(m)
		if !ok || !found {
			return nil, fmt.Errorf("base schema %s is not loaded", m)
		}
		isPolicy := mmeta.SchemaType == SchemaTypePolicy
		if !isPolicy && p.Type == "" {
			p.Type = m
		}
		if m != name && isPolicy && p.Extends == "" && p.Type == "" {
			p.Extends = m
		}
		p.members = append(p.members, member{name: m, value: value, isPolicy: isPolicy})
	}
	if p.Type == "" {
		return nil, fmt.Errorf("base_schema chain %v does not reach a type schema", p.Chain)
	}

	b := newTreeBuilder()
	for i := len(p.members) - 1; i >= 0; i-- {
		m := p.members[i]
		if err := b.merge(b.root, m.value, nil, m.name, m.isPolicy); err != nil {
			return nil, err
		}
	}
	if err := checkAnnotations(p, b.root, b.rules); err != nil {
		return nil, err
	}
	p.tree = b.root
	for _, r := range b.rules {
		p.Rules = append(p.Rules, *r)
	}
	sort.Slice(p.Rules, func(i, j int) bool { return p.Rules[i].Name < p.Rules[j].Name })

	if err := checkSensitive(p, sensitive); err != nil {
		return nil, err
	}
	digest, err := digestOf(p.members)
	if err != nil {
		return nil, err
	}
	p.Digest = digest
	return p, nil
}

// checkSensitive rejects a policy that constrains a sensitive path or anything
// beneath one: those values are redacted before storage, so the policy could
// never evaluate them.
func checkSensitive(p *Policy, sensitive *redact.Registry) error {
	paths, err := sensitive.For(p.Name)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	var visit func(n *node, segs []string) error
	visit = func(n *node, segs []string) error {
		for _, seg := range n.order {
			c := n.children[seg]
			cs := append(append([]string(nil), segs...), seg)
			if c.fromPolicy {
				fp, err := pathOf(cs)
				if err != nil {
					return fmt.Errorf("field path %v: %w", cs, err)
				}
				for _, s := range paths {
					if fp.HasPrefix(s) {
						return fmt.Errorf("constrains %s, which is declared in sensitive_fields and redacted before evaluation", fp.String())
					}
				}
			}
			if err := visit(c, cs); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(p.tree, nil)
}

// Policies returns every loaded policy, sorted by name.
func (s *Set) Policies() []*Policy {
	out := make([]*Policy, 0, len(s.policies))
	for _, p := range s.policies {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Policy returns a loaded policy by name.
func (s *Set) Policy(name string) (*Policy, bool) {
	p, ok := s.policies[schemaname.Normalize(name)]
	return p, ok
}

// Applicable returns the policies whose chain reaches typeName or one of its
// ancestors, sorted by name.
func (s *Set) Applicable(typeName string) []*Policy {
	typeName = schemaname.Normalize(typeName)
	var out []*Policy
	for _, p := range s.Policies() {
		if s.applies(p, typeName) {
			out = append(out, p)
		}
	}
	return out
}

// Review returns a type schema's review; a schema that is not loaded is a
// draft.
func (s *Set) Review(typeName string) validator.SchemaReview {
	if meta, ok := s.src.GetSchemaMetadata(schemaname.Normalize(typeName)); ok {
		return meta.Review
	}
	return validator.SchemaReview{Status: validator.ReviewDraft}
}
