# Schema Inference Algorithm

This document describes how PUDL's schema inference engine automatically determines which CUE schema best matches incoming data.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     Schema Inference Flow                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Data Input ──► Heuristic Scoring ──► Candidate Selection ──►   │
│                                                                  │
│  ──► CUE Unification ──► Confidence Calculation ──► Result      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Core Components

Located in `internal/inference/`:

### 1. SchemaInferrer (`inference.go`)

The main orchestrator that coordinates the inference process:

```go
type SchemaInferrer struct {
    loader    *validator.CUEModuleLoader
    modules   map[string]*validator.LoadedModule
    schemas   map[string]cue.Value
    metadata  map[string]validator.SchemaMetadata
    graph     *InheritanceGraph
}
```

### 2. InheritanceGraph (`graph.go`)

Tracks schema parent-child relationships for specificity ordering:

```go
type InheritanceGraph struct {
    children map[string][]string // parent -> children (more specific)
    parents  map[string]string   // child -> parent (less specific)
    all      map[string]bool     // all schema names
    roots    []string            // base schemas (no parent)
    leaves   []string            // most specific (no children)
}
```

- Built from `base_schema` field in `_pudl` metadata
- `GetMostSpecificFirst()` returns schemas ordered by depth (leaves first)
- Used to prioritize more specific schemas before generic ones

### 3. Heuristics (`heuristics.go`)

Pre-filters and scores candidate schemas before expensive CUE unification:

```go
type InferenceHints struct {
    Origin         string // e.g., "aws-ec2-instances"
    Format         string // e.g., "json", "yaml"
    CollectionType string // "collection", "item", or ""
}
```

## The Inference Algorithm

```
1. EXTRACT top-level field names from incoming data

2. SELECT CANDIDATES:
   For each schema with metadata:
   a. Check collection type compatibility (list vs item)
   b. Score identity_fields matches (+0.5 if all present)
   c. Score tracked_fields matches (+0.1 × ratio)
   d. Score origin hints vs resource_type (+0.15)
   e. Include catchall with minimal score (+0.01)

3. SORT candidates by:
   a. Heuristic score (descending)
   b. Inheritance depth (more specific first)
   c. Alphabetical (deterministic tiebreaker)

4. TRY CUE UNIFICATION for each candidate:
   a. Convert data to CUE value
   b. Call schema.Unify(dataValue)
   c. For lists: Validate() (lenient for disjunctions)
   d. For structs: Validate(cue.Concrete(true)) (strict)

5. RETURN first matching schema with confidence score

6. FALLBACK to catchall if nothing matches
```

## Heuristic Scoring

| Factor | Score Boost | Description |
|--------|-------------|-------------|
| All identity fields present | +0.5 | Strong match indicator |
| Partial identity fields | +0.2 × ratio | Weaker indicator |
| Tracked fields present | +0.1 × ratio | Supporting evidence |
| Origin matches resource_type | +0.15 | Requires 2+ keyword matches |
| Catchall fallback | +0.01 | Always included as last resort |
| List-type for collections | +0.02 | Structural match |

## Schema Metadata (`_pudl`)

Schemas include embedded metadata that drives inference:

```go
type SchemaMetadata struct {
    SchemaType     string   // "base", "policy", "custom", "catchall"
    ResourceType   string   // "aws.ec2.instance", "k8s.pod"
    BaseSchema     string   // Parent schema reference
    IdentityFields []string // Key fields for this type
    TrackedFields  []string // Fields to monitor for changes
    IsListType     bool     // Structurally a list (auto-detected)
}
```

### Field Purposes

| Field | Purpose | Example |
|-------|---------|---------|
| `identity_fields` | Uniquely identify a resource; strong inference signal | `["InstanceId"]` |
| `tracked_fields` | Monitor for changes (future use); weak inference signal | `["State", "Tags"]` |
| `resource_type` | Match against origin hints | `"aws.ec2.instance"` |
| `base_schema` | Build inheritance graph for specificity | `"aws.#BaseResource"` |
| (alphabetical) | Deterministic tiebreaker when scores/depth are equal | N/A |

## CUE Unification

The `tryUnify` method validates data against a schema:

```go
func (si *SchemaInferrer) tryUnify(schema cue.Value, jsonBytes []byte) bool {
    ctx := schema.Context()
    dataValue := ctx.CompileBytes(jsonBytes)
    unified := schema.Unify(dataValue)
    
    // List types need lenient validation (disjunctions)
    if isListType {
        return unified.Validate() == nil
    }
    // Structs need concrete validation
    return unified.Validate(cue.Concrete(true)) == nil
}
```

## Confidence Calculation

```go
confidence = heuristicScore + positionBoost

// Position boost rewards early matches (more specific schemas)
positionBoost = (totalCandidates - matchPosition) / totalCandidates * 0.3

// Bounds: 0.1 ≤ confidence ≤ 1.0
```

## Key Design Decisions

1. **Heuristics-first approach** - Cheap field matching narrows candidates before expensive CUE validation
2. **Most-specific-first ordering** - Leaf schemas (deepest inheritance) tried first
3. **Never-reject philosophy** - Always returns a schema, falling back to catchall
4. **Structural list detection** - Uses CUE's `IncompleteKind()` rather than relying solely on metadata
5. **Thread-safe** - Uses `sync.RWMutex` for concurrent access
6. **Hot reload support** - `Reload()` method allows runtime schema updates

