package executor

import (
	"pudl/internal/definition"
)

// resolveArgs builds the arguments map for a method execution from a definition.
// It extracts socket bindings and merges in any tags from RunOptions.
func (e *Executor) resolveArgs(def *definition.DefinitionInfo, tags map[string]string) map[string]interface{} {
	args := make(map[string]interface{})

	// Include socket bindings as args
	for k, v := range def.SocketBindings {
		args[k] = v
	}

	// Include definition metadata
	args["_definition"] = def.Name
	args["_model"] = def.ModelRef

	// Merge tags (overrides socket bindings if same key)
	for k, v := range tags {
		args[k] = v
	}

	return args
}
