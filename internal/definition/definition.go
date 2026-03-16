package definition

// DefinitionInfo represents a named instance with concrete configuration.
type DefinitionInfo struct {
	Name           string            // e.g., "prod_instance"
	SchemaRef      string            // e.g., "examples.#EC2Instance"
	Package        string            // CUE package name
	FilePath       string            // Source file
	SocketBindings map[string]string // input socket -> source expression
	Validated      bool              // Whether it passed validation
}

// ValidationResult holds the result of validating a definition.
type ValidationResult struct {
	Name   string   // Definition name
	Valid  bool     // Whether validation passed
	Errors []string // Validation error messages
}
