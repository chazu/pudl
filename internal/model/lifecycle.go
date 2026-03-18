package model

import "fmt"

// Lifecycle represents the execution order for a method invocation,
// including any qualification methods that must pass first.
type Lifecycle struct {
	Qualifications []string // Qualification methods to run before the action
	Action         string   // The primary method being invoked
	PostActions    []string // Attribute/codegen methods to run after
}

// ResolveLifecycle determines the execution order for a given method.
// It finds all qualification methods that block the requested method,
// and any attribute/codegen methods that should run after.
func ResolveLifecycle(model *ModelInfo, methodName string) (*Lifecycle, error) {
	method, ok := model.Methods[methodName]
	if !ok {
		return nil, fmt.Errorf("method %q not found in model %s", methodName, model.Name)
	}

	lifecycle := &Lifecycle{
		Action: methodName,
	}

	// Find qualifications that block this method
	for name, m := range model.Methods {
		if m.Kind != "qualification" {
			continue
		}
		for _, blocked := range m.Blocks {
			if blocked == methodName {
				lifecycle.Qualifications = append(lifecycle.Qualifications, name)
				break
			}
		}
	}

	// Find post-action methods (attribute and codegen) that should run after
	// actions but not after qualifications
	if method.Kind == "action" {
		for name, m := range model.Methods {
			if m.Kind == "attribute" || m.Kind == "codegen" {
				lifecycle.PostActions = append(lifecycle.PostActions, name)
			}
		}
	}

	return lifecycle, nil
}
