package model

// ModelInfo represents a parsed model with all its components.
type ModelInfo struct {
	Name     string        // e.g., "examples.#EC2InstanceModel"
	Package  string        // e.g., "pudl/model/examples"
	FilePath string        // Source file path
	Schema   string        // Referenced schema identifier
	State    string        // Referenced state schema (if any)
	Metadata ModelMetadata // Model metadata
	Methods  map[string]Method
	Sockets  map[string]Socket
	Auth     *AuthConfig
}

// ModelMetadata holds descriptive information about a model.
type ModelMetadata struct {
	Name        string
	Description string
	Category    string
	Icon        string
}

// Method represents a model method declaration.
type Method struct {
	Kind        string   // "action", "qualification", "attribute", "codegen"
	Description string
	Timeout     string
	Retries     int
	Blocks      []string // for qualifications: which methods this gates
}

// Socket represents a typed input/output port on a model.
type Socket struct {
	Direction   string // "input" or "output"
	Description string
	Required    bool
}

// AuthConfig holds authentication configuration for a model.
type AuthConfig struct {
	Method string // "bearer", "sigv4", "basic", "custom"
}
