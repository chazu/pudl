package inference

import (
	"encoding/json"
	"fmt"
	"sync"

	"cuelang.org/go/cue"

	"pudl/internal/validator"
)

// SchemaInferrer determines the best matching schema for data by attempting
// CUE unification against schemas from the schema repository.
type SchemaInferrer struct {
	mu        sync.RWMutex
	loader    *validator.CUEModuleLoader
	modules   map[string]*validator.LoadedModule
	schemas   map[string]cue.Value
	metadata  map[string]validator.SchemaMetadata
	graph     *InheritanceGraph
	ctx       *cue.Context
	schemaPath string
}

// InferenceResult represents the result of schema inference.
type InferenceResult struct {
	Schema      string   // Best matching schema name
	Confidence  float64  // 0.0-1.0 confidence score
	CascadePath []string // Schemas tried in order
	MatchedAt   int      // Index in cascade path where match occurred (-1 if catchall)
	Reason      string   // Why this schema was selected
}

// NewSchemaInferrer creates a new schema inferrer that loads schemas from the given path.
func NewSchemaInferrer(schemaPath string) (*SchemaInferrer, error) {
	loader := validator.NewCUEModuleLoader(schemaPath)

	modules, err := loader.LoadAllModules()
	if err != nil {
		return nil, fmt.Errorf("failed to load CUE modules: %w", err)
	}

	schemas := loader.GetAllSchemas(modules)
	metadata := loader.GetAllMetadata(modules)
	graph := BuildInheritanceGraph(metadata)

	return &SchemaInferrer{
		loader:     loader,
		modules:    modules,
		schemas:    schemas,
		metadata:   metadata,
		graph:      graph,
		schemaPath: schemaPath,
	}, nil
}

// Infer determines the best matching schema for the given data.
// It uses heuristics to select candidate schemas, then tries CUE unification
// starting with the most specific candidates.
func (si *SchemaInferrer) Infer(data interface{}, hints InferenceHints) (*InferenceResult, error) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	if len(si.schemas) == 0 {
		return &InferenceResult{
			Schema:     "core.#CatchAll",
			Confidence: 0.1,
			Reason:     "no schemas loaded",
		}, nil
	}

	// Get candidate schemas using heuristics
	candidates := SelectCandidates(data, hints, si.metadata, si.graph)

	// If no candidates from heuristics, use all schemas sorted by specificity
	if len(candidates) == 0 {
		allSchemas := si.graph.GetMostSpecificFirst()
		for _, schema := range allSchemas {
			candidates = append(candidates, CandidateScore{Schema: schema, Score: 0.1})
		}
	}

	// Convert data to CUE-compatible JSON
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data to JSON: %w", err)
	}

	// Build cascade path from candidates
	cascadePath := make([]string, 0, len(candidates))
	for _, c := range candidates {
		cascadePath = append(cascadePath, c.Schema)
	}

	// Try each candidate schema
	for i, candidate := range candidates {
		schema, exists := si.schemas[candidate.Schema]
		if !exists {
			continue
		}

		// Skip the catchall for now - we'll use it as final fallback
		if isCatchallSchema(candidate.Schema) {
			continue
		}

		// Attempt CUE unification
		if si.tryUnify(schema, jsonBytes) {
			confidence := calculateConfidence(candidate.Score, i, len(candidates))
			return &InferenceResult{
				Schema:      candidate.Schema,
				Confidence:  confidence,
				CascadePath: cascadePath,
				MatchedAt:   i,
				Reason:      candidate.Reason,
			}, nil
		}
	}

	// No schema matched - return appropriate fallback based on collection type
	fallbackSchema := findFallbackSchema(si.schemas, si.metadata, hints.CollectionType)
	return &InferenceResult{
		Schema:      fallbackSchema,
		Confidence:  0.1,
		CascadePath: cascadePath,
		MatchedAt:   -1,
		Reason:      "no schema matched, using fallback",
	}, nil
}

// tryUnify attempts to unify data with a schema using CUE.
// Returns true if the data validates against the schema.
func (si *SchemaInferrer) tryUnify(schema cue.Value, jsonBytes []byte) bool {
	// Get the CUE context from the schema
	ctx := schema.Context()
	if ctx == nil {
		return false
	}

	// Compile the JSON data as a CUE value
	dataValue := ctx.CompileBytes(jsonBytes)
	if dataValue.Err() != nil {
		return false
	}

	// Unify the schema with the data
	unified := schema.Unify(dataValue)

	// First check if unification itself failed (structural mismatch)
	if unified.Err() != nil {
		return false
	}

	// For schemas with disjunctions (like collection schemas with unions),
	// Validate with Concrete(true) will fail because CUE can't pick a branch.
	// Instead, we first try without Concrete to see if the structure matches,
	// then try with Concrete for schemas that don't have disjunctions.
	//
	// Check if the schema is a list type - list types with element constraints
	// often have disjunctions and need more lenient validation.
	isListType := (schema.IncompleteKind() & cue.ListKind) != 0

	if isListType {
		// For list types, just check that unification succeeded without errors.
		// The disjunction in the element type (e.g., [...(A | B | C)]) will cause
		// Concrete validation to fail even when the data is valid.
		return unified.Validate() == nil
	}

	// For non-list types, use Concrete(true) to ensure all required fields
	// have concrete values in the data.
	return unified.Validate(cue.Concrete(true)) == nil
}

// Reload reloads schemas from the schema repository.
func (si *SchemaInferrer) Reload() error {
	si.mu.Lock()
	defer si.mu.Unlock()

	modules, err := si.loader.LoadAllModules()
	if err != nil {
		return fmt.Errorf("failed to reload CUE modules: %w", err)
	}

	si.modules = modules
	si.schemas = si.loader.GetAllSchemas(modules)
	si.metadata = si.loader.GetAllMetadata(modules)
	si.graph = BuildInheritanceGraph(si.metadata)

	return nil
}

// GetAvailableSchemas returns the names of all loaded schemas.
func (si *SchemaInferrer) GetAvailableSchemas() []string {
	si.mu.RLock()
	defer si.mu.RUnlock()

	schemas := make([]string, 0, len(si.schemas))
	for name := range si.schemas {
		schemas = append(schemas, name)
	}
	return schemas
}

// GetSchemaMetadata returns metadata for a specific schema.
func (si *SchemaInferrer) GetSchemaMetadata(schemaName string) (validator.SchemaMetadata, bool) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	meta, exists := si.metadata[schemaName]
	return meta, exists
}

// GetInheritanceGraph returns the schema inheritance graph.
func (si *SchemaInferrer) GetInheritanceGraph() *InheritanceGraph {
	si.mu.RLock()
	defer si.mu.RUnlock()

	return si.graph
}

// calculateConfidence computes a confidence score based on heuristic score and match position.
func calculateConfidence(heuristicScore float64, matchPosition, totalCandidates int) float64 {
	// Base confidence from heuristics (0.0-1.0)
	confidence := heuristicScore

	// Boost if matched early in the cascade (more specific schemas come first)
	if totalCandidates > 1 {
		positionBoost := float64(totalCandidates-matchPosition) / float64(totalCandidates) * 0.3
		confidence += positionBoost
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	// Floor at 0.1 for any successful match
	if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}

// isCatchallSchema checks if a schema name represents a catchall schema.
func isCatchallSchema(schemaName string) bool {
	return schemaName == "core.#CatchAll" ||
		schemaName == "pudl.schemas/pudl/core:#CatchAll" ||
		schemaName == "pudl/core.#CatchAll"
}

// findCatchallSchema finds the catchall schema name from available schemas.
func findCatchallSchema(schemas map[string]cue.Value) string {
	// Try common catchall names
	catchallNames := []string{
		"core.#CatchAll",
		"pudl.schemas/pudl/core:#CatchAll",
		"pudl/core.#CatchAll",
	}

	for _, name := range catchallNames {
		if _, exists := schemas[name]; exists {
			return name
		}
	}

	// Search for any schema with "CatchAll" in the name
	for name := range schemas {
		if isCatchallSchema(name) || containsCatchAll(name) {
			return name
		}
	}

	// Default fallback
	return "core.#CatchAll"
}

// containsCatchAll checks if a schema name contains "CatchAll".
func containsCatchAll(name string) bool {
	return len(name) >= 8 && (name[len(name)-8:] == "CatchAll" ||
		(len(name) >= 9 && name[len(name)-9:] == "#CatchAll"))
}

// findFallbackSchema finds an appropriate fallback schema based on collection type.
// For collections, it returns a collection-appropriate schema instead of the item catchall.
func findFallbackSchema(schemas map[string]cue.Value, metadata map[string]validator.SchemaMetadata, collectionType string) string {
	if collectionType == "collection" {
		// For collections, try to find a collection-type fallback
		collectionFallbacks := []string{
			"pudl.schemas/pudl/core:#Collection",
			"core.#Collection",
			"pudl/core.#Collection",
		}

		for _, name := range collectionFallbacks {
			if _, exists := schemas[name]; exists {
				return name
			}
		}

		// Search for any list-type schema as fallback (using structural detection, not metadata)
		for name, meta := range metadata {
			if meta.IsListType {
				return name
			}
		}
	}

	// Default to item catchall for items or unknown types
	return findCatchallSchema(schemas)
}
