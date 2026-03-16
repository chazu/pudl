// Package typepattern provides type detection patterns for common data formats.
// It enables automatic detection of data types like Kubernetes resources, AWS configs,
// and GitLab CI pipelines based on field patterns and expected values.
package typepattern

// TypePattern defines a pattern for detecting a specific type of data.
// Patterns are matched against data fields to determine the type with a confidence score.
type TypePattern struct {
	// Name identifies the pattern, e.g., "kubernetes", "aws-ec2"
	Name string

	// Ecosystem groups related patterns, e.g., "kubernetes", "aws", "gitlab"
	Ecosystem string

	// RequiredFields lists fields that must be present for this pattern to match
	RequiredFields []string

	// OptionalFields lists fields that boost confidence when present
	OptionalFields []string

	// FieldValues maps field names to expected values.
	// If a field has one of the expected values, confidence increases.
	FieldValues map[string][]string

	// TypeExtractor extracts the specific type identifier from matched data.
	// For Kubernetes, this might return "batch/v1:Job" from apiVersion and kind fields.
	TypeExtractor func(data map[string]interface{}) string

	// ImportMapper maps a type ID to its CUE import path.
	// For Kubernetes, this might map "batch/v1:Job" to "cue.dev/x/k8s.io/api/batch/v1"
	ImportMapper func(typeID string) string

	// MetadataDefaults returns default PUDL metadata for this pattern.
	MetadataDefaults func(typeID string) *PudlMetadata

	// Priority determines pattern check order. Higher values are checked first.
	Priority int
}

// DetectedType represents the result of type detection.
type DetectedType struct {
	// Pattern is the TypePattern that matched the data
	Pattern *TypePattern

	// TypeID is the specific type identifier, e.g., "batch/v1:Job"
	TypeID string

	// ImportPath is the CUE import path, e.g., "cue.dev/x/k8s.io/api/batch/v1"
	ImportPath string

	// Definition is the type definition name, e.g., "Job"
	Definition string

	// Confidence is the match confidence score from 0.0 to 1.0
	Confidence float64
}

// PudlMetadata holds PUDL-specific metadata for a detected type.
type PudlMetadata struct {
	// SchemaType categorizes the schema, e.g., "resource", "config", "collection"
	SchemaType string

	// ResourceType identifies the resource type, e.g., "kubernetes-job", "aws-ec2-instance"
	ResourceType string

	// IdentityFields lists fields used to uniquely identify a resource
	IdentityFields []string

	// TrackedFields lists fields that should be tracked for change detection
	TrackedFields []string
}
