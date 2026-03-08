package executor

import (
	"fmt"
	"os"
	"strings"

	"pudl/internal/definition"
)

const vaultPrefix = "vault://"

// resolveArgs builds the arguments map for a method execution from a definition.
// It extracts socket bindings and merges in any tags from RunOptions.
// String values starting with "vault://" are resolved via the vault.
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

	// Resolve vault references
	if e.vault != nil {
		for k, v := range args {
			if s, ok := v.(string); ok && strings.HasPrefix(s, vaultPrefix) {
				path := s[len(vaultPrefix):]
				resolved, err := e.vault.Get(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to resolve vault secret %q: %v\n", path, err)
				} else {
					args[k] = resolved
				}
			}
		}
	}

	return args
}
