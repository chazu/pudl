package inference

import (
	cueerrors "cuelang.org/go/cue/errors"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"cuelang.org/go/cue"

	"github.com/chazu/pudl/internal/schemaname"
	"github.com/chazu/pudl/internal/validator"
)

// SchemaInferrer determines the best matching schema for data by attempting
// CUE unification against schemas from the schema repository.
type SchemaInferrer struct {
	mu          sync.RWMutex
	loaders     []*validator.CUEModuleLoader
	modules     map[string]*validator.LoadedModule
	schemas     map[string]cue.Value
	metadata    map[string]validator.SchemaMetadata
	graph       *InheritanceGraph
	schemaPaths []string
	loadErrors  []validator.SchemaLoadError
	cycles      [][]string
	sources     map[string]string
	shadowed    map[string][]string
}

// InferenceResult represents the result of schema inference.
type InferenceResult struct {
	Schema      string          // Best matching schema name
	Confidence  float64         // 0.0-1.0 confidence score
	CascadePath []string        // Schemas tried in order
	MatchedAt   int             // Index in cascade path where match occurred (-1 if catchall)
	Reason      string          // Why this schema was selected
	Trace       *InferenceTrace `json:"trace,omitempty"`
}

// NewSchemaInferrer creates a new schema inferrer that loads schemas from the given path(s).
// When multiple paths are provided, schemas are loaded in order; the first occurrence
// of a schema name wins (per-repo shadows global).
func NewSchemaInferrer(schemaPaths ...string) (*SchemaInferrer, error) {
	if len(schemaPaths) == 0 {
		return nil, fmt.Errorf("at least one schema path is required")
	}

	si := &SchemaInferrer{schemaPaths: schemaPaths}
	si.apply(validator.LoadSchemaSet(schemaPaths))
	return si, nil
}

// apply installs a freshly loaded schema set. Loading uses first-found-wins
// shadowing: earlier paths take priority, so a per-repo schema shadows a global
// one of the same name. Callers hold si.mu for writing, or own si exclusively.
func (si *SchemaInferrer) apply(set *validator.SchemaSet) {
	si.loaders = set.Loaders
	si.modules = set.Modules
	si.schemas = set.Schemas
	si.metadata = set.Metadata
	si.graph = BuildInheritanceGraph(set.Metadata)
	si.loadErrors = set.Errors
	si.cycles = set.Cycles
	si.sources = set.Sources
	si.shadowed = set.Shadowed
}

// LoadErrors reports the schema packages that failed to load, and loaded
// schemas that failed integrity checks. Data that one of them would have matched
// falls back to the catch-all, so callers should surface these.
func (si *SchemaInferrer) LoadErrors() []validator.SchemaLoadError {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return append([]validator.SchemaLoadError(nil), si.loadErrors...)
}

// BaseSchemaCycles reports base_schema cycles among the loaded schemas.
func (si *SchemaInferrer) BaseSchemaCycles() [][]string {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return append([][]string(nil), si.cycles...)
}

// Infer determines the best matching schema for the given data.
// It uses heuristics to select candidate schemas, then tries CUE unification
// starting with the most specific candidates.
func (si *SchemaInferrer) Infer(data interface{}, hints InferenceHints) (*InferenceResult, error) {
	return si.infer(data, hints, false)
}

// InferWithTrace explains the existing cascade without changing its ordering.
func (si *SchemaInferrer) InferWithTrace(data interface{}, hints InferenceHints) (*InferenceResult, error) {
	return si.infer(data, hints, true)
}
func (si *SchemaInferrer) infer(data interface{}, hints InferenceHints, explain bool) (*InferenceResult, error) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	var trace *InferenceTrace
	if explain {
		trace = si.traceLocked()
	}
	finish := func(result *InferenceResult) (*InferenceResult, error) {
		if trace != nil {
			trace.Selected = schemaname.Normalize(result.Schema)
			trace.Reason = result.Reason
			trace.Fallback = result.MatchedAt < 0 || len(si.schemas) == 0
			result.Trace = trace
		}
		return result, nil
	}
	if len(si.schemas) == 0 {
		return finish(&InferenceResult{
			Schema:     "core.#Item",
			Confidence: 0.1,
			Reason:     "no schemas loaded",
		})
	}

	// Get candidate schemas using heuristics
	candidates := SelectCandidates(data, hints, si.metadata, si.graph)

	// If no candidates from heuristics, use all schemas sorted by specificity
	if len(candidates) == 0 {
		allSchemas := si.graph.GetMostSpecificFirst()
		for _, schema := range allSchemas {
			candidates = append(candidates, CandidateScore{Schema: schema, Score: 0.1, Reason: "all loaded schemas ordered by specificity"})
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

	// The data is compiled once per CUE context and reused across candidates;
	// schemas loaded from the same search path share a context.
	dataByCtx := make(map[*cue.Context]cue.Value)

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
		unifyErr := si.unifyError(schema, jsonBytes, dataByCtx)
		if trace != nil {
			attempt := CandidateAttempt{Schema: schemaname.Normalize(candidate.Schema), Score: candidate.Score, Reason: candidate.Reason, Matched: unifyErr == nil, SourcePath: si.sources[candidate.Schema], ShadowedPaths: si.shadowed[candidate.Schema]}
			if unifyErr != nil {
				attempt.FailurePaths = boundedFailurePaths(unifyErr)
			}
			if len(trace.Attempts) < 64 {
				trace.Attempts = append(trace.Attempts, attempt)
			} else {
				trace.AttemptsTruncated = true
			}
		}
		if unifyErr == nil {
			if trace != nil {
				trace.Alternatives = si.tiedAlternatives(candidates, i, jsonBytes, dataByCtx)
			}
			confidence := calculateConfidence(candidate.Score, i, len(candidates))
			return finish(&InferenceResult{
				Schema:      candidate.Schema,
				Confidence:  confidence,
				CascadePath: cascadePath,
				MatchedAt:   i,
				Reason:      candidate.Reason,
			})
		}
	}

	// No schema matched - return appropriate fallback based on collection type
	fallbackSchema := findFallbackSchema(si.schemas, si.metadata, hints.CollectionType)
	return finish(&InferenceResult{
		Schema:      fallbackSchema,
		Confidence:  0.1,
		CascadePath: cascadePath,
		MatchedAt:   -1,
		Reason:      "no schema matched, using fallback",
	})
}

// tryUnify attempts to unify data with a schema using CUE.
// Returns true if the data validates against the schema. dataByCtx memoizes
// the compiled data per CUE context for the duration of one Infer call.

func (si *SchemaInferrer) unifyError(schema cue.Value, jsonBytes []byte, dataByCtx map[*cue.Context]cue.Value) error {
	// Get the CUE context from the schema
	ctx := schema.Context()
	if ctx == nil {
		return fmt.Errorf("schema context unavailable")
	}

	// Compile the JSON data as a CUE value, once per context
	dataValue, compiled := dataByCtx[ctx]
	if !compiled {
		dataValue = ctx.CompileBytes(jsonBytes)
		dataByCtx[ctx] = dataValue
	}
	if dataValue.Err() != nil {
		return dataValue.Err()
	}

	// Unify the schema with the data
	unified := schema.Unify(dataValue)

	// First check if unification itself failed (structural mismatch)
	if unified.Err() != nil {
		return unified.Err()
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
		return unified.Validate()
	}

	// For non-list types, use Concrete(true) to ensure all required fields
	// have concrete values in the data.
	return unified.Validate(cue.Concrete(true))
}

// Reload reloads schemas from the schema repository.
func (si *SchemaInferrer) Reload() error {
	si.mu.Lock()
	defer si.mu.Unlock()

	si.apply(validator.LoadSchemaSet(si.schemaPaths))

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

// GetSchemaValue returns the loaded CUE value for a canonical schema name.
// Binding resolution uses it to authorize the exact projected source field.
func (si *SchemaInferrer) GetSchemaValue(schemaName string) (cue.Value, bool) {
	si.mu.RLock()
	defer si.mu.RUnlock()

	value, exists := si.schemas[schemaname.Normalize(schemaName)]
	return value, exists
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

// isCatchallSchema checks if a schema name represents a catchall/fallback schema.
// Uses the schemaname package for normalization-aware comparison.
func isCatchallSchema(name string) bool {
	return schemaname.IsFallbackSchema(name)
}

// findCatchallSchema finds the catchall schema name from available schemas.
func findCatchallSchema(schemas map[string]cue.Value) string {
	// Canonical fallback schema name
	const fallbackCanonical = "pudl/core.#Item"

	// Check if canonical name exists
	if _, exists := schemas[fallbackCanonical]; exists {
		return fallbackCanonical
	}

	// Search for any schema that is a fallback
	for name := range schemas {
		if schemaname.IsFallbackSchema(name) {
			return name
		}
	}

	// Default fallback (canonical format)
	return fallbackCanonical
}

// findFallbackSchema finds an appropriate fallback schema based on collection type.
// For collections, it returns a collection-appropriate schema instead of the item catchall.
func findFallbackSchema(schemas map[string]cue.Value, metadata map[string]validator.SchemaMetadata, collectionType string) string {
	if collectionType == "collection" {
		// Canonical collection fallback
		const collectionFallback = "pudl/core.#Collection"

		if _, exists := schemas[collectionFallback]; exists {
			return collectionFallback
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

// CandidateAttempt includes field paths only; rejected values are never copied.
type CandidateAttempt struct {
	Schema        string   `json:"schema"`
	Score         float64  `json:"score"`
	Reason        string   `json:"reason"`
	Matched       bool     `json:"matched"`
	FailurePaths  []string `json:"failure_paths,omitempty"`
	SourcePath    string   `json:"source_path,omitempty"`
	ShadowedPaths []string `json:"shadowed_paths,omitempty"`
}
type InferenceTrace struct {
	Selected          string             `json:"selected"`
	Reason            string             `json:"reason"`
	Fallback          bool               `json:"fallback"`
	ScoreKind         string             `json:"score_kind"`
	Attempts          []CandidateAttempt `json:"attempts"`
	AttemptsTruncated bool               `json:"attempts_truncated,omitempty"`
	LoadFailures      []string           `json:"load_failure_paths,omitempty"`
	Historical        bool               `json:"historical,omitempty"`
	// Alternatives are other schemas, outside the selected one's family, that
	// tied with it on score and also accepted the data: the classification was
	// decided by candidate order alone. Computed only when tracing.
	Alternatives []string `json:"alternatives,omitempty"`
}

func (si *SchemaInferrer) traceLocked() *InferenceTrace {
	trace := &InferenceTrace{ScoreKind: "heuristic score, not a calibrated probability", Attempts: []CandidateAttempt{}}
	for i, err := range si.loadErrors {
		if i == 16 {
			break
		}
		trace.LoadFailures = append(trace.LoadFailures, err.Path)
	}
	return trace
}
func boundedFailurePaths(err error) []string {
	set := map[string]bool{}
	for _, item := range cueerrors.Errors(err) {
		path := strings.Join(item.Path(), ".")
		if path == "" {
			path = "<root>"
		}
		set[path] = true
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) > 16 {
		paths = paths[:16]
	}
	return paths
}

// SchemaSource reports the winning search path and paths shadowed by it.
func (si *SchemaInferrer) SchemaSource(name string) (string, []string) {
	si.mu.RLock()
	defer si.mu.RUnlock()
	name = schemaname.Normalize(name)
	return si.sources[name], append([]string(nil), si.shadowed[name]...)
}
