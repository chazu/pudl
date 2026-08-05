package systemmodel

// BindingClass is the explicit @pudl(binding=...) channel classification.
type BindingClass string

const (
	BindingPlain  BindingClass = "plain"
	BindingSealed BindingClass = "sealed"
)

// ValueBinding selects one scalar field from one exact typed catalog resource.
type ValueBinding struct {
	Source ResourceRef `json:"source"`
	Path   string      `json:"path"`
}

// ResourceRef identifies a resource inside a producer model observation.
type ResourceRef struct {
	Model    string         `json:"model"`
	Schema   string         `json:"schema"`
	Identity map[string]any `json:"identity"`
}

// SealedSource names a model-wide unique sealed output on a producer model.
type SealedSource struct {
	Model  string `json:"model"`
	Output string `json:"output"`
}

// SealedInput is phase-owned metadata. Exactly one of Ref or Source is set.
// Ref is intentionally omitted from JSON so catalog/report serialization cannot
// accidentally persist the provider path.
type SealedInput struct {
	Ref          string        `json:"-"`
	Source       *SealedSource `json:"source,omitempty"`
	DeliveryMode string        `json:"delivery_mode"`
}

// SealedOutput is phase-owned metadata. Ref is operational data and is never
// part of the default JSON projection.
type SealedOutput struct {
	Ref       string `json:"-"`
	StoreMode string `json:"store_mode"`
}
