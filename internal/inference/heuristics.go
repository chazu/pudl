package inference

import (
	"strings"

	"pudl/internal/validator"
)

// InferenceHints provides optional context to improve candidate selection.
type InferenceHints struct {
	Origin         string // e.g., "aws-ec2-instances", "kubectl-get-pods"
	Format         string // e.g., "json", "yaml", "csv"
	CollectionType string // "collection", "item", or "" for unknown
	DeclaredSchema string // value of "_schema" field in the data, if present
}

// CandidateScore represents a schema and its heuristic score.
type CandidateScore struct {
	Schema string
	Score  float64
	Reason string
}

// SelectCandidates returns schemas that are likely to match the given data,
// ordered by likelihood (highest score first). This narrows down the schemas
// to try before expensive CUE unification.
func SelectCandidates(
	data interface{},
	hints InferenceHints,
	metadata map[string]validator.SchemaMetadata,
	graph *InheritanceGraph,
) []CandidateScore {
	dataFields := extractTopLevelFields(data)

	// Extract _schema from data if not already provided in hints
	if hints.DeclaredSchema == "" {
		hints.DeclaredSchema = extractDeclaredSchema(data)
	}

	var candidates []CandidateScore

	for schemaName, meta := range metadata {
		score, reason := scoreCandidate(schemaName, meta, dataFields, hints)
		if score > 0 {
			candidates = append(candidates, CandidateScore{
				Schema: schemaName,
				Score:  score,
				Reason: reason,
			})
		}
	}

	// Sort by:
	// 1. Score (descending)
	// 2. Inheritance depth (more specific first)
	// 3. Cascade priority (higher first)
	sortCandidates(candidates, graph)

	return candidates
}

// scoreCandidate calculates a heuristic score for how likely a schema matches the data.
// Returns 0 if the schema definitely won't match.
func scoreCandidate(
	schemaName string,
	meta validator.SchemaMetadata,
	dataFields map[string]bool,
	hints InferenceHints,
) (float64, string) {
	score := 0.0
	var reasons []string

	// Filter by collection type if specified
	// Collections should only match collection schemas (list types), items should not match collection schemas.
	// We use IsListType (structural detection from CUE) rather than relying on metadata,
	// since list-type schemas like `#CatchAllCollection: [...]` can't have metadata.
	if hints.CollectionType != "" {
		if hints.CollectionType == "collection" {
			// Collections should only match list-type schemas
			if !meta.IsListType {
				return 0, "schema type mismatch (need collection/list schema)"
			}
		} else if hints.CollectionType == "item" {
			// Items should not match list-type schemas
			if meta.IsListType {
				return 0, "schema type mismatch (item cannot use collection/list schema)"
			}
		}
	}

	// Check _schema field - if the data declares its own schema, this is the
	// strongest possible hint. Match it against resource_type in schema metadata.
	if hints.DeclaredSchema != "" && meta.ResourceType != "" {
		if strings.EqualFold(hints.DeclaredSchema, meta.ResourceType) {
			score += 0.9
			reasons = append(reasons, "_schema field matches resource_type")
			return score, strings.Join(reasons, ", ")
		}
	}

	// Check identity fields - these are strong indicators
	if len(meta.IdentityFields) > 0 {
		matchedIdentity := 0
		for _, field := range meta.IdentityFields {
			// Handle nested fields like "metadata.name" by checking top-level
			topLevel := strings.Split(field, ".")[0]
			if dataFields[topLevel] {
				matchedIdentity++
			}
		}

		if matchedIdentity == len(meta.IdentityFields) {
			// All identity fields present - strong match
			score += 0.5
			reasons = append(reasons, "all identity fields present")
		} else if matchedIdentity > 0 {
			// Partial match - some indicator
			score += 0.2 * float64(matchedIdentity) / float64(len(meta.IdentityFields))
			reasons = append(reasons, "some identity fields present")
		}
	}

	// Check tracked fields - weaker indicators but still useful
	if len(meta.TrackedFields) > 0 {
		matchedTracked := 0
		for _, field := range meta.TrackedFields {
			topLevel := strings.Split(field, ".")[0]
			if dataFields[topLevel] {
				matchedTracked++
			}
		}

		if matchedTracked > 0 {
			score += 0.1 * float64(matchedTracked) / float64(len(meta.TrackedFields))
			reasons = append(reasons, "tracked fields present")
		}
	}

	// Check origin hints from resource_type
	if hints.Origin != "" && meta.ResourceType != "" {
		originLower := strings.ToLower(hints.Origin)
		resourceLower := strings.ToLower(meta.ResourceType)

		// Split both into parts for matching
		originParts := strings.FieldsFunc(originLower, func(r rune) bool {
			return r == '-' || r == '_' || r == '.' || r == '/'
		})
		resourceParts := strings.FieldsFunc(resourceLower, func(r rune) bool {
			return r == '-' || r == '_' || r == '.' || r == '/'
		})

		// Count how many meaningful parts match
		// Exclude generic terms like "aws", "k8s" alone - require service-level match
		matchCount := 0
		for _, originPart := range originParts {
			// Normalize plurals (simple: remove trailing 's')
			normalized := strings.TrimSuffix(originPart, "s")
			for _, resourcePart := range resourceParts {
				if originPart == resourcePart || normalized == resourcePart {
					matchCount++
					break
				}
			}
		}

		// Require at least 2 parts to match for a meaningful origin hint
		// This prevents "aws" alone from matching all AWS schemas
		if matchCount >= 2 {
			score += 0.15
			reasons = append(reasons, "origin matches resource type")
		}
	}

	// Catchall always gets a minimal score so it's always considered
	if meta.SchemaType == "catchall" || strings.Contains(schemaName, "CatchAll") {
		if score == 0 {
			score = 0.01
			reasons = append(reasons, "catchall fallback")
		}
	}

	// List-type schemas (collections) without metadata should still be considered
	// when inferring collection data. Give them a minimal score so they're tried
	// via CUE unification. This allows schemas like `#Ec2InstanceCollection: [...#Ec2Instance]`
	// to be matched even without _pudl metadata.
	if meta.IsListType && hints.CollectionType == "collection" && score == 0 {
		score = 0.02 // Slightly higher than catchall so specific collections are tried first
		reasons = append(reasons, "list-type schema for collection data")
	}

	reason := strings.Join(reasons, ", ")
	if reason == "" {
		reason = "no specific match"
	}

	return score, reason
}

// sortCandidates sorts candidates by score, then by specificity, then by priority.
func sortCandidates(candidates []CandidateScore, graph *InheritanceGraph) {
	// Use a simple bubble sort for now - candidate lists are typically small
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if shouldSwap(candidates[i], candidates[j], graph) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
}

// shouldSwap returns true if candidate j should come before candidate i.
func shouldSwap(i, j CandidateScore, graph *InheritanceGraph) bool {
	// Primary: score (higher first)
	if j.Score > i.Score {
		return true
	}
	if i.Score > j.Score {
		return false
	}

	// Secondary: inheritance depth (more specific first)
	depthI := graph.calculateDepth(i.Schema)
	depthJ := graph.calculateDepth(j.Schema)
	if depthJ > depthI {
		return true
	}
	if depthI > depthJ {
		return false
	}

	// Tertiary: alphabetical (deterministic, A before Z)
	return j.Schema < i.Schema
}

// extractTopLevelFields extracts the top-level field names from data.
func extractTopLevelFields(data interface{}) map[string]bool {
	fields := make(map[string]bool)

	switch d := data.(type) {
	case map[string]interface{}:
		for key := range d {
			fields[key] = true
		}
	case []interface{}:
		// For arrays, extract fields from the first element
		if len(d) > 0 {
			if first, ok := d[0].(map[string]interface{}); ok {
				for key := range first {
					fields[key] = true
				}
			}
		}
	}

	return fields
}

// extractDeclaredSchema returns the value of the "_schema" field if present in
// a map, or "" otherwise. This supports data that self-declares its schema type.
func extractDeclaredSchema(data interface{}) string {
	m, ok := data.(map[string]interface{})
	if !ok {
		return ""
	}
	v, ok := m["_schema"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// GetFieldList returns a sorted list of field names from data.
// Useful for debugging and logging.
func GetFieldList(data interface{}) []string {
	fields := extractTopLevelFields(data)
	var result []string
	for field := range fields {
		result = append(result, field)
	}
	// Sort for deterministic output
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}
