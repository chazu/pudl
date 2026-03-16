// Package typepattern provides type detection patterns for common data formats.
package typepattern

import "strings"

// kubernetesImportMappings maps type IDs (apiVersion:kind) to CUE import paths.
// The key is the type ID, value is [importPath, definitionName].
var kubernetesImportMappings = map[string][2]string{
	// batch/v1
	"batch/v1:Job":     {"cue.dev/x/k8s.io/api/batch/v1", "Job"},
	"batch/v1:CronJob": {"cue.dev/x/k8s.io/api/batch/v1", "CronJob"},

	// apps/v1
	"apps/v1:Deployment":  {"cue.dev/x/k8s.io/api/apps/v1", "Deployment"},
	"apps/v1:StatefulSet": {"cue.dev/x/k8s.io/api/apps/v1", "StatefulSet"},
	"apps/v1:DaemonSet":   {"cue.dev/x/k8s.io/api/apps/v1", "DaemonSet"},
	"apps/v1:ReplicaSet":  {"cue.dev/x/k8s.io/api/apps/v1", "ReplicaSet"},

	// core/v1 (apiVersion is just "v1")
	"v1:Pod":                   {"cue.dev/x/k8s.io/api/core/v1", "Pod"},
	"v1:Service":               {"cue.dev/x/k8s.io/api/core/v1", "Service"},
	"v1:ConfigMap":             {"cue.dev/x/k8s.io/api/core/v1", "ConfigMap"},
	"v1:Secret":                {"cue.dev/x/k8s.io/api/core/v1", "Secret"},
	"v1:ServiceAccount":        {"cue.dev/x/k8s.io/api/core/v1", "ServiceAccount"},
	"v1:PersistentVolumeClaim": {"cue.dev/x/k8s.io/api/core/v1", "PersistentVolumeClaim"},

	// networking.k8s.io/v1
	"networking.k8s.io/v1:Ingress":       {"cue.dev/x/k8s.io/api/networking/v1", "Ingress"},
	"networking.k8s.io/v1:NetworkPolicy": {"cue.dev/x/k8s.io/api/networking/v1", "NetworkPolicy"},

	// rbac.authorization.k8s.io/v1
	"rbac.authorization.k8s.io/v1:Role":        {"cue.dev/x/k8s.io/api/rbac/v1", "Role"},
	"rbac.authorization.k8s.io/v1:ClusterRole": {"cue.dev/x/k8s.io/api/rbac/v1", "ClusterRole"},
}

// kubernetesTrackedFields maps kind to fields that should be tracked for change detection.
var kubernetesTrackedFields = map[string][]string{
	"Job":         {"status.succeeded", "status.failed", "status.active"},
	"CronJob":     {"status.lastScheduleTime", "status.lastSuccessfulTime"},
	"Deployment":  {"status.replicas", "status.readyReplicas", "status.availableReplicas"},
	"StatefulSet": {"status.replicas", "status.readyReplicas"},
	"DaemonSet":   {"status.numberReady", "status.desiredNumberScheduled"},
	"ReplicaSet":  {"status.replicas", "status.readyReplicas"},
	"Pod":         {"status.phase", "status.conditions"},
	"Service":     {"status.loadBalancer"},
	"Ingress":     {"status.loadBalancer"},
}

// extractKubernetesTypeID extracts a type identifier from Kubernetes data.
// Returns a string in the format "apiVersion:kind", e.g., "batch/v1:Job".
func extractKubernetesTypeID(data map[string]interface{}) string {
	apiVersion, ok1 := data["apiVersion"].(string)
	kind, ok2 := data["kind"].(string)
	if !ok1 || !ok2 {
		return ""
	}
	return apiVersion + ":" + kind
}

// mapKubernetesImport maps a Kubernetes type ID to its CUE import path.
// Returns the import path with the definition name appended, or empty string if unknown.
func mapKubernetesImport(typeID string) string {
	if mapping, ok := kubernetesImportMappings[typeID]; ok {
		return mapping[0] + ":" + mapping[1]
	}
	// Return empty string for unknown types to allow fallback
	return ""
}

// kubernetesMetadataDefaults returns default PUDL metadata for a Kubernetes type.
func kubernetesMetadataDefaults(typeID string) *PudlMetadata {
	// Extract kind from typeID (e.g., "batch/v1:Job" -> "Job")
	kind := extractDefinition(typeID)

	// Extract apiGroup from typeID (e.g., "batch/v1:Job" -> "batch")
	apiGroup := extractAPIGroup(typeID)

	// Build resource type: k8s.<apiGroup>.<kind_lowercase>
	resourceType := "k8s." + apiGroup + "." + strings.ToLower(kind)

	// Get tracked fields for this kind, or use default
	trackedFields := kubernetesTrackedFields[kind]
	if trackedFields == nil {
		trackedFields = []string{}
	}

	return &PudlMetadata{
		SchemaType:      "kubernetes",
		ResourceType:    resourceType,
		IdentityFields: []string{"metadata.name", "metadata.namespace"},
		TrackedFields:   trackedFields,
	}
}

// extractAPIGroup extracts the API group from a type ID.
// For "batch/v1:Job" returns "batch", for "v1:Pod" returns "core".
func extractAPIGroup(typeID string) string {
	// Find the colon that separates apiVersion from kind
	colonIdx := strings.Index(typeID, ":")
	if colonIdx == -1 {
		return "core"
	}

	apiVersion := typeID[:colonIdx]

	// Check if there's a slash (group/version) or just version
	slashIdx := strings.Index(apiVersion, "/")
	if slashIdx == -1 {
		// No group, it's core (e.g., "v1")
		return "core"
	}

	// Extract group (e.g., "batch" from "batch/v1")
	group := apiVersion[:slashIdx]

	// Handle complex groups like "networking.k8s.io" -> "networking"
	dotIdx := strings.Index(group, ".")
	if dotIdx != -1 {
		group = group[:dotIdx]
	}

	return group
}

// BuildKubernetesDetectedType builds a DetectedType from a kind and apiVersion.
// This is used for CLI commands that need to generate schemas for known Kubernetes types.
// Returns the DetectedType with pattern, typeID, importPath, and definition set.
func BuildKubernetesDetectedType(kind, apiVersion string) *DetectedType {
	typeID := apiVersion + ":" + kind
	importPath := mapKubernetesImport(typeID)

	// Split importPath:Definition format back to just import path
	actualImportPath := ""
	if importPath != "" {
		colonIdx := len(importPath) - len(kind) - 1
		if colonIdx > 0 && importPath[colonIdx] == ':' {
			actualImportPath = importPath[:colonIdx]
		}
	}

	return &DetectedType{
		Pattern: &TypePattern{
			Name:             "kubernetes",
			Ecosystem:        "kubernetes",
			MetadataDefaults: kubernetesMetadataDefaults,
		},
		TypeID:     typeID,
		ImportPath: actualImportPath,
		Definition: kind,
		Confidence: 1.0,
	}
}

// GetKnownKubernetesTypes returns a list of known Kubernetes type IDs.
// This can be used for CLI suggestions.
func GetKnownKubernetesTypes() []string {
	types := make([]string, 0, len(kubernetesImportMappings))
	for typeID := range kubernetesImportMappings {
		types = append(types, typeID)
	}
	return types
}

// RegisterKubernetesPatterns registers Kubernetes type detection patterns with the registry.
func RegisterKubernetesPatterns(r *Registry) {
	r.Register(&TypePattern{
		Name:           "kubernetes",
		Ecosystem:      "kubernetes",
		RequiredFields: []string{"apiVersion", "kind", "metadata"},
		OptionalFields: []string{"spec", "status", "data"},
		FieldValues: map[string][]string{
			"kind": {
				"Pod", "Deployment", "Service", "ConfigMap", "Secret",
				"Job", "CronJob", "StatefulSet", "DaemonSet", "ReplicaSet",
				"Ingress", "NetworkPolicy", "Role", "ClusterRole",
				"ServiceAccount", "PersistentVolumeClaim",
			},
		},
		TypeExtractor:    extractKubernetesTypeID,
		ImportMapper:     mapKubernetesImport,
		MetadataDefaults: kubernetesMetadataDefaults,
		Priority:         100,
	})
}
