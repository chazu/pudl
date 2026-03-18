package glojure

import "fmt"

// RegisterBuiltins registers all builtin namespaces (pudl.core, pudl.http) into
// the registry, mapping them to CUE op.#Name identifiers.
func RegisterBuiltins(registry *Registry) error {
	// Register Go functions into Glojure namespaces
	if err := registerCoreNamespace(registry); err != nil {
		return fmt.Errorf("registering core namespace: %w", err)
	}
	if err := registerHTTPNamespace(registry); err != nil {
		return fmt.Errorf("registering http namespace: %w", err)
	}

	// Map CUE function names to registry entries.
	// Cacheable functions return deterministic results for the same args.
	cueBindings := []struct {
		cueName   string
		ns        string
		fn        string
		cacheable bool
	}{
		// pudl.core
		{"#Uppercase", "pudl.core", "uppercase", true},
		{"#Lowercase", "pudl.core", "lowercase", true},
		{"#Concat", "pudl.core", "concat", true},
		{"#Format", "pudl.core", "format", true},
		{"#Now", "pudl.core", "now", false},
		{"#Env", "pudl.core", "env", false},
		// pudl.http
		{"#HttpGet", "pudl.http", "get", false},
		{"#HttpGetJson", "pudl.http", "get-json", false},
		{"#HttpPost", "pudl.http", "post", false},
		{"#HttpStatus", "pudl.http", "status", false},
	}

	for _, b := range cueBindings {
		registry.RegisterGlojure(b.cueName, b.ns, b.fn, WithCacheable(b.cacheable))
	}

	return nil
}
