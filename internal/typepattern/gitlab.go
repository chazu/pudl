// Package typepattern provides type detection patterns for common data formats.
package typepattern

// gitlabCIReservedKeys are top-level keys that are NOT job definitions in GitLab CI.
var gitlabCIReservedKeys = map[string]bool{
	"stages":    true,
	"variables": true,
	"default":   true,
	"workflow":  true,
	"include":   true,
	"image":     true,
	"services":  true,
	"before_script": true,
	"after_script":  true,
	"cache":     true,
}

// isGitLabJob checks if a value looks like a GitLab CI job definition.
// Jobs are objects containing script, extends, or trigger fields.
func isGitLabJob(v interface{}) bool {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return false
	}
	_, hasScript := obj["script"]
	_, hasExtends := obj["extends"]
	_, hasTrigger := obj["trigger"]
	return hasScript || hasExtends || hasTrigger
}

// detectGitLabCI checks if the data looks like a GitLab CI pipeline.
// Returns true if it finds job-like structures in the data.
func detectGitLabCI(data map[string]interface{}) bool {
	// Check for stages array (common but optional)
	if _, hasStages := data["stages"]; hasStages {
		// Look for job definitions
		for key, value := range data {
			if gitlabCIReservedKeys[key] {
				continue
			}
			if isGitLabJob(value) {
				return true
			}
		}
	}

	// Check for jobs with script directly (pipelines without explicit stages)
	jobCount := 0
	for key, value := range data {
		if gitlabCIReservedKeys[key] {
			continue
		}
		if isGitLabJob(value) {
			jobCount++
		}
	}
	return jobCount >= 1
}

// extractGitLabCITypeID extracts a type identifier from GitLab CI data.
// GitLab CI is a single type, so this always returns "gitlab-ci:Pipeline".
func extractGitLabCITypeID(data map[string]interface{}) string {
	if !detectGitLabCI(data) {
		return ""
	}
	return "gitlab-ci:Pipeline"
}

// mapGitLabCIImport maps a GitLab CI type ID to its CUE import path.
func mapGitLabCIImport(typeID string) string {
	if typeID == "gitlab-ci:Pipeline" {
		return "cue.dev/x/gitlab/gitlabci:Pipeline"
	}
	return ""
}

// gitlabCIMetadataDefaults returns default PUDL metadata for a GitLab CI type.
func gitlabCIMetadataDefaults(typeID string) *PudlMetadata {
	return &PudlMetadata{
		SchemaType:      "cicd",
		ResourceType:    "gitlab.pipeline",
		CascadePriority: 85,
		IdentityFields:  []string{},
		TrackedFields:   []string{"stages"},
	}
}

// RegisterGitLabPatterns registers GitLab CI type detection patterns with the registry.
func RegisterGitLabPatterns(r *Registry) {
	r.Register(&TypePattern{
		Name:      "gitlab-ci",
		Ecosystem: "gitlab",
		// GitLab CI doesn't have simple required fields - detection is based on
		// finding job-like structures. We use an empty RequiredFields and rely
		// on the TypeExtractor to validate the structure.
		RequiredFields: []string{},
		OptionalFields: []string{"stages", "variables", "default", "workflow", "include"},
		TypeExtractor: func(data map[string]interface{}) string {
			return extractGitLabCITypeID(data)
		},
		ImportMapper:     mapGitLabCIImport,
		MetadataDefaults: gitlabCIMetadataDefaults,
		Priority:         70,
	})
}

