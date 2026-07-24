// Package acute contains the PUDL-owned ACUTE run policy.
//
// mu remains responsible for executing plugin/toolchain actions. This package
// owns the run lifecycle around those operations: resolving scope, choosing the
// next phase, and interpreting the result of a verified re-observation.
package acute

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/idgen"
	"github.com/chazu/pudl/internal/systemmodel"
)

// RunRequest is the policy input for one ACUTE run. It intentionally contains
// no subprocess or catalog details; those belong to adapters at the run seam.
type RunRequest struct {
	Converge    bool
	Only        []string
	DryRun      bool
	MaxIters    int
	FromCatalog bool
}

// RunPlan is the resolved, side-effect-free plan for one run. Effective is the
// model every scope-sensitive phase must consume; Original is retained for
// identity and reporting.
type RunPlan struct {
	Original  *systemmodel.SystemModel
	Effective *systemmodel.SystemModel
	Request   RunRequest
}

// RunSession gives one resolved plan a durable audit identity. The first
// session implementation is intentionally audit-only; resume/recovery is a
// separate state-machine decision.
type RunSession struct {
	RunID string
	Plan  *RunPlan
}

// NewRunSession starts an audit-identified run after its side-effect-free plan
// has been resolved.
func NewRunSession(plan *RunPlan) *RunSession {
	return &RunSession{
		RunID: "run_" + idgen.GenerateRandomProquint(),
		Plan:  plan,
	}
}

// NewRunPlan validates run policy and resolves --only before any external
// process or catalog write can occur.
func NewRunPlan(model *systemmodel.SystemModel, request RunRequest) (*RunPlan, error) {
	if model == nil {
		return nil, fmt.Errorf("run plan needs a model")
	}
	if request.Converge {
		if request.MaxIters < 1 {
			return nil, fmt.Errorf("--max-iters must be >= 1")
		}
	} else {
		switch {
		case len(request.Only) > 0:
			return nil, fmt.Errorf("--only requires --converge")
		case request.DryRun:
			return nil, fmt.Errorf("--dry-run requires --converge")
		}
	}

	effective, err := ScopeModelForRun(model, request.Only)
	if err != nil {
		return nil, err
	}
	return &RunPlan{Original: model, Effective: effective, Request: request}, nil
}

// ScopeModelForRun applies --only to desired resources and includes their
// declared resource dependencies transitively. Selectors match a resource's
// schema/definition, identity name, path, id, kind, or metadata.name. Short
// schema names are accepted in addition to their canonical _schema value.
func ScopeModelForRun(model *systemmodel.SystemModel, selectors []string) (*systemmodel.SystemModel, error) {
	if len(selectors) == 0 {
		return model, nil
	}
	if !model.Convergent() {
		return nil, fmt.Errorf("--only requires a convergent model")
	}

	wanted := make([]string, 0, len(selectors))
	seen := map[string]bool{}
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return nil, fmt.Errorf("--only contains an empty selector")
		}
		if !seen[selector] {
			seen[selector] = true
			wanted = append(wanted, selector)
		}
	}

	selectedIndexes := map[int]bool{}
	var unknown []string
	for _, selector := range wanted {
		indexes, err := resolveSelector(model.Desired, selector)
		if err != nil {
			return nil, fmt.Errorf("--only %w", err)
		}
		if len(indexes) == 0 {
			unknown = append(unknown, selector)
			continue
		}
		for _, index := range indexes {
			selectedIndexes[index] = true
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("--only selector(s) did not match desired resources: %s", strings.Join(unknown, ", "))
	}

	for changed := true; changed; {
		changed = false
		for index := range selectedIndexes {
			for _, dependency := range desiredDependencies(model.Desired[index]) {
				dependencyIndex, err := resolveDependency(model.Desired, dependency)
				if err != nil {
					return nil, err
				}
				if !selectedIndexes[dependencyIndex] {
					selectedIndexes[dependencyIndex] = true
					changed = true
				}
			}
		}
	}

	selected := make([]map[string]any, 0, len(selectedIndexes))
	for index, desired := range model.Desired {
		if selectedIndexes[index] {
			selected = append(selected, desired)
		}
	}

	scoped := *model
	scoped.Desired = selected
	return &scoped, nil
}

func desiredDependencies(desired map[string]any) []string {
	var dependencies []string
	for _, key := range []string{"depends_on", "dependsOn"} {
		switch value := desired[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				dependencies = append(dependencies, strings.TrimSpace(value))
			}
		case []string:
			for _, dependency := range value {
				if strings.TrimSpace(dependency) != "" {
					dependencies = append(dependencies, strings.TrimSpace(dependency))
				}
			}
		case []any:
			for _, dependency := range value {
				if s := strings.TrimSpace(fmt.Sprint(dependency)); s != "" && s != "<nil>" {
					dependencies = append(dependencies, s)
				}
			}
		}
	}
	return dependencies
}

// selectorKind records how a selector string matched one desired resource. The
// distinction matters because the two classes have different cardinality
// contracts: an identity selector names exactly one resource, while a type
// selector legitimately names a set (`--only Deployment`). Collapsing them into
// one namespace lets a selector capture resources the operator never named.
type selectorKind struct {
	identity bool
	typed    bool
}

// typeSelectorKeys name a resource's type, so matching several resources is the
// documented intent. identitySelectorKeys name one resource.
var (
	typeSelectorKeys     = []string{"_schema", "schema", "definition", "kind"}
	identitySelectorKeys = []string{"name", "id", "path", "target"}
)

// resolveSelector returns the indexes of the desired resources a selector names,
// or nil when it matches nothing. A selector must resolve unambiguously: either
// it identifies one resource by an identity key, or it selects a set by a type
// key — never a mix of the two, and never several resources by identity.
// Resolving an ambiguous selector silently would put resources the operator did
// not name into converge scope.
func resolveSelector(desired []map[string]any, selector string) ([]int, error) {
	var identityMatches, typeMatches []int
	for index, resource := range desired {
		kind, ok := desiredSelectorValues(resource)[selector]
		if !ok {
			continue
		}
		if kind.identity {
			identityMatches = append(identityMatches, index)
		}
		if kind.typed {
			typeMatches = append(typeMatches, index)
		}
	}

	switch {
	case len(identityMatches) == 0 && len(typeMatches) == 0:
		return nil, nil
	case len(identityMatches) > 1:
		return nil, fmt.Errorf("selector %q matches %d resources by identity (%s); selectors must be unique",
			selector, len(identityMatches), describeResources(desired, identityMatches, selector, identitySelectorKeys))
	case len(identityMatches) == 1 && len(typeMatches) > 0 && !sameIndexes(identityMatches, typeMatches):
		return nil, fmt.Errorf("selector %q is ambiguous: it names %s by identity and also matches %s by type",
			selector,
			describeResources(desired, identityMatches, selector, identitySelectorKeys),
			describeResources(desired, typeMatches, selector, typeSelectorKeys))
	case len(identityMatches) == 1:
		return identityMatches, nil
	default:
		return typeMatches, nil
	}
}

// resolveDependency resolves one declared dependency to exactly one desired
// resource. Unlike a user-supplied selector, a dependency edge points at a
// single resource, so a set match is an error rather than a set selection.
func resolveDependency(desired []map[string]any, dependency string) (int, error) {
	indexes, err := resolveSelector(desired, dependency)
	if err != nil {
		return 0, fmt.Errorf("--only dependency %w", err)
	}
	switch len(indexes) {
	case 0:
		return 0, fmt.Errorf("--only dependency selector %q did not match a desired resource", dependency)
	case 1:
		return indexes[0], nil
	default:
		return 0, fmt.Errorf("--only dependency selector %q matches %d resources (%s); a dependency must name exactly one",
			dependency, len(indexes), describeResources(desired, indexes, dependency, append(append([]string{}, identitySelectorKeys...), typeSelectorKeys...)))
	}
}

func sameIndexes(a, b []int) bool {
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

// describeResources renders matched resources for an error message. Each label
// names the resource by its most specific identifier, then states which key
// produced the match when that is not already obvious — an operator reading
// "matches name=decoy by type" needs to be told it was the `kind` field.
func describeResources(desired []map[string]any, indexes []int, selector string, keys []string) string {
	labels := make([]string, 0, len(indexes))
	for _, index := range indexes {
		label := describeResource(desired[index])
		if key := matchingKey(desired[index], selector, keys); key != "" && !strings.HasPrefix(label, key+"=") {
			label += " via " + key
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

// matchingKey reports which of keys on this resource yields the selector,
// accounting for the short name accepted after a schema's '#'.
func matchingKey(desired map[string]any, selector string, keys []string) string {
	for _, key := range keys {
		value, ok := desired[key]
		if !ok {
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(value))
		if s == selector {
			return key
		}
		if hash := strings.LastIndexByte(s, '#'); hash >= 0 && hash+1 < len(s) && s[hash+1:] == selector {
			return key
		}
	}
	return ""
}

func describeResource(desired map[string]any) string {
	for _, key := range append(append([]string{}, identitySelectorKeys...), typeSelectorKeys...) {
		if value, ok := desired[key]; ok {
			if s := strings.TrimSpace(fmt.Sprint(value)); s != "" && s != "<nil>" {
				return key + "=" + s
			}
		}
	}
	if metadata, ok := desired["metadata"].(map[string]any); ok {
		if name, ok := metadata["name"]; ok {
			if s := strings.TrimSpace(fmt.Sprint(name)); s != "" && s != "<nil>" {
				return "metadata.name=" + s
			}
		}
	}
	return "<unnamed resource>"
}

func desiredSelectorValues(desired map[string]any) map[string]selectorKind {
	values := make(map[string]selectorKind)
	add := func(value any, typed bool) {
		s := strings.TrimSpace(fmt.Sprint(value))
		if s == "" || s == "<nil>" {
			return
		}
		mark := func(v string) {
			kind := values[v]
			if typed {
				kind.typed = true
			} else {
				kind.identity = true
			}
			values[v] = kind
		}
		mark(s)
		if hash := strings.LastIndexByte(s, '#'); hash >= 0 && hash+1 < len(s) {
			mark(s[hash+1:])
		}
	}
	for _, key := range typeSelectorKeys {
		if value, ok := desired[key]; ok {
			add(value, true)
		}
	}
	for _, key := range identitySelectorKeys {
		if value, ok := desired[key]; ok {
			add(value, false)
		}
	}
	if metadata, ok := desired["metadata"].(map[string]any); ok {
		if name, ok := metadata["name"]; ok {
			add(name, false)
		}
	}
	return values
}
