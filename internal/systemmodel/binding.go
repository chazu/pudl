package systemmodel

// BindingClass is the explicit @pudl(binding=...) channel classification.
// Only plain fields may feed a value binding; a sealed field is never
// bindable (sealed value delivery itself was retired 2026-11-01).
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
