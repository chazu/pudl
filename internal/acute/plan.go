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
	// SnapshotID is allocated with the run, not by the ingest that writes it.
	// PUDL decides when to observe, so it owns the identifier for the
	// observation it initiates — which is what lets a run name the snapshot a
	// failed ingest would have produced, instead of discarding the ID with the
	// error and leaving the partial state unnamed.
	SnapshotID string
	Plan       *RunPlan
}

// NewRunSession starts an audit-identified run after its side-effect-free plan
// has been resolved.
func NewRunSession(plan *RunPlan) *RunSession {
	return &RunSession{
		RunID:      "run_" + idgen.GenerateRandomProquint(),
		SnapshotID: "snap_" + idgen.GenerateRandomProquint(),
		Plan:       plan,
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

// desiredDependencies reads a desired resource's declared dependencies.
//
// One spelling, one shape: `depends_on` as a list of selector strings, which is
// what `#DesiredResource` declares. The previously-accepted `dependsOn` alias
// and bare-string forms are gone — an untyped field with three spellings is the
// ambiguity D4 asked to remove, and CUE now rejects the shapes this no longer
// reads, so a model using them fails loudly at load instead of silently having
// its dependency ignored.
func desiredDependencies(desired map[string]any) []string {
	var dependencies []string
	switch value := desired["depends_on"].(type) {
	case []string:
		for _, dependency := range value {
			if trimmed := strings.TrimSpace(dependency); trimmed != "" {
				dependencies = append(dependencies, trimmed)
			}
		}
	case []any:
		// CUE and JSON both decode a list into []any.
		for _, dependency := range value {
			if s := strings.TrimSpace(fmt.Sprint(dependency)); s != "" && s != "<nil>" {
				dependencies = append(dependencies, s)
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
// resource, by an IDENTITY key only.
//
// A dependency edge points at a single resource, so unlike a user-supplied
// `--only` selector it may not name a type. A type key that happens to match one
// resource today is still the wrong thing to write: adding a second resource of
// that type would silently turn one edge into two, which is the Defect 2 failure
// mode one level down. Rejecting the *class* rather than the cardinality is what
// makes the rule stable as the model grows.
func resolveDependency(desired []map[string]any, dependency string) (int, error) {
	var identityMatches, typeMatches []int
	for index, resource := range desired {
		kind, ok := desiredSelectorValues(resource)[dependency]
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
	case len(identityMatches) == 1 && len(typeMatches) > 0 && !sameIndexes(identityMatches, typeMatches):
		// Identity would win, but silently preferring it is what Defect 2 rejected:
		// the author cannot tell which resource they got, and the two readings
		// select different sets. Erroring keeps the fix rather than softening it
		// into a rule of thumb.
		return 0, fmt.Errorf("depends_on %q is ambiguous: it names %s by identity and also matches %s by type",
			dependency,
			describeResources(desired, identityMatches, dependency, identitySelectorKeys),
			describeResources(desired, typeMatches, dependency, typeSelectorKeys))
	case len(identityMatches) == 1:
		return identityMatches[0], nil
	case len(identityMatches) > 1:
		return 0, fmt.Errorf("depends_on %q matches %d resources by identity (%s); a dependency must name exactly one",
			dependency, len(identityMatches),
			describeResources(desired, identityMatches, dependency, identitySelectorKeys))
	case len(typeMatches) > 0:
		return 0, fmt.Errorf("depends_on %q names a type, not a resource (it matches %s); a dependency must name one resource by an identity key (name, id, path, target or metadata.name)",
			dependency, describeResources(desired, typeMatches, dependency, typeSelectorKeys))
	default:
		return 0, fmt.Errorf("depends_on %q did not match a desired resource", dependency)
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
