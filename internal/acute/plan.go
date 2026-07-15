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

	matched := map[string]bool{}
	for _, desired := range model.Desired {
		values := desiredSelectorValues(desired)
		for _, selector := range wanted {
			if values[selector] {
				matched[selector] = true
			}
		}
	}

	var unknown []string
	for _, selector := range wanted {
		if !matched[selector] {
			unknown = append(unknown, selector)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("--only selector(s) did not match desired resources: %s", strings.Join(unknown, ", "))
	}

	selectedIndexes := map[int]bool{}
	for index, desired := range model.Desired {
		values := desiredSelectorValues(desired)
		for _, selector := range wanted {
			if values[selector] {
				selectedIndexes[index] = true
			}
		}
	}

	for changed := true; changed; {
		changed = false
		for index := range selectedIndexes {
			for _, dependency := range desiredDependencies(model.Desired[index]) {
				dependencyIndex := -1
				for candidateIndex, candidate := range model.Desired {
					if desiredSelectorValues(candidate)[dependency] {
						dependencyIndex = candidateIndex
						break
					}
				}
				if dependencyIndex < 0 {
					return nil, fmt.Errorf("--only dependency selector %q did not match a desired resource", dependency)
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

func desiredSelectorValues(desired map[string]any) map[string]bool {
	values := make(map[string]bool)
	add := func(value any) {
		s := strings.TrimSpace(fmt.Sprint(value))
		if s == "" || s == "<nil>" {
			return
		}
		values[s] = true
		if hash := strings.LastIndexByte(s, '#'); hash >= 0 && hash+1 < len(s) {
			values[s[hash+1:]] = true
		}
	}
	for _, key := range []string{"_schema", "schema", "definition", "name", "id", "path", "kind", "target"} {
		if value, ok := desired[key]; ok {
			add(value)
		}
	}
	if metadata, ok := desired["metadata"].(map[string]any); ok {
		if name, ok := metadata["name"]; ok {
			add(name)
		}
	}
	return values
}
