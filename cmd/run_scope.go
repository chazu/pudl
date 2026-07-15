package cmd

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/systemmodel"
)

// scopeModelForRun applies `run --converge --only` to the desired resources.
// Selectors are exact resource selectors: a resource's schema/definition,
// identity name, path, id, kind, or metadata.name. Short schema names are
// accepted in addition to their canonical `_schema` value.
func scopeModelForRun(model *systemmodel.SystemModel, selectors []string) (*systemmodel.SystemModel, error) {
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

	// Include declared resource dependencies transitively. V1 models usually
	// have model-level DependsOn only, but desired records may carry a
	// depends_on/dependsOn selector list; honor it when present.
	selected := make([]map[string]any, 0, len(model.Desired))
	selectedIndexes := map[int]bool{}
	for index, desired := range model.Desired {
		for _, selector := range wanted {
			if desiredSelectorValues(desired)[selector] {
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
