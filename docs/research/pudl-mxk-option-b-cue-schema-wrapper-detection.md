---
id: pudl-mxk
title: "Robust implementation of CUE schema-based collection wrapper detection (Option B)"
type: research
status: complete
date: 2026-02-18
tags: [schema, collection, inference, cue, wrapper, typepattern]
depends_on: [pudl-yqt]
---

# Research: Option B — CUE Schema-Based Collection Wrapper Detection

## Executive Summary

Option B (built-in CUE schema `#CollectionWrapper`) **cannot work as a standalone solution** because CUE's type system fundamentally cannot express the structural constraint "has at least one unknown-named field that is an array of objects." However, a **hybrid approach** combining a family of CUE schemas with TypePattern-based Go detection is viable and arguably superior to pure Option A. This document details the CUE limitations, explores hybrid designs, and makes a concrete recommendation.

---

## 1. CUE Structural Constraint Capabilities

### What CUE CAN Express

| Feature | Syntax | Example |
|---------|--------|---------|
| Open structs | `...` | `#Def: { name: string, ... }` |
| Typed open structs | `...T` | `{ ...string }` — all extra fields must be strings |
| Pattern constraints | `[=~"regex"]: T` | `[=~"^data_"]: [...{...}]` |
| Disjunctions | `A \| B` | `"ndjson" \| "json-array"` |
| List constraints | `[...T]` | `[...{id: string}]` — array of objects with id |
| Length constraints | `list.MinItems(l, n)` | Minimum list length |
| matchN | `matchN(n, [...])` | Count how many constraints a value satisfies |
| Optional fields | `field?: T` | Field may or may not exist |

### What CUE CANNOT Express (Critical Limitations)

1. **"At least one field is an array of objects" (unknown field name)**: Pattern constraints like `[string]: [...{...}]` apply to ALL fields uniformly. You cannot say "some fields are arrays, others are scalars" — CUE would reject the scalar fields.

2. **Runtime type introspection in comprehensions**: Cannot write `if typeof(val) == "list"` inside a comprehension. Guards work only on concrete values, not type checks.

3. **Cardinality over structural patterns**: Cannot express "exactly one field matches pattern X" without enumerating all possible field names in a disjunction.

4. **Negation**: Cannot express "not an array" directly — no negation operator for types.

5. **Mixed-type open structs**: Cannot say "additional fields can be arrays OR scalars, but at least one must be an array." The `...` ellipsis accepts everything or constrains everything uniformly.

### Concrete Demonstration of the Limitation

To match `{"items": [...], "count": 2}`, you'd need:

```cue
// IMPOSSIBLE in pure CUE — this forces ALL fields to be arrays:
#WrapperGeneric: {
    [string]: [...{...}]  // ← "count": 2 would FAIL this
}

// ONLY works if you know the key name:
#WrapperItems: {
    items: [...{...}]
    ...                   // ← allows "count": 2 as extra field
}
```

This is the fundamental blocker: a generic `#CollectionWrapper` that matches any wrapper key name is impossible in pure CUE.

---

## 2. Option B Variants Explored

### Variant B1: Single Generic CUE Schema (INFEASIBLE)

As shown above, CUE cannot express the structural pattern without knowing the array field name. A single `#CollectionWrapper` schema is not possible.

### Variant B2: Family of CUE Schemas (One Per Known Key Name)

Define one schema per known wrapper key name:

```cue
#WrapperItems: {
    _pudl: { schema_type: "collection_wrapper", resource_type: "generic.collection_wrapper.items", cascade_priority: 50 }
    items: [...{...}]
    ...
}

#WrapperData: {
    _pudl: { schema_type: "collection_wrapper", resource_type: "generic.collection_wrapper.data", cascade_priority: 50 }
    data: [...{...}]
    ...
}

#WrapperResults: {
    _pudl: { schema_type: "collection_wrapper", resource_type: "generic.collection_wrapper.results", cascade_priority: 50 }
    results: [...{...}]
    ...
}
// ... ~15 more for known key names
```

**Pros:**
- Uses existing CUE unification infrastructure — no new Go code for matching
- Each schema is simple and correct — `items: [...{...}]` with open struct works
- Priority system handles ordering naturally
- Schemas can have identity_fields (the array key) and tracked_fields (pagination keys)
- Users can add custom schemas for their API's wrapper key names

**Cons:**
- ~15-30 nearly identical schema definitions (one per key name variant, case-sensitive)
- Misses domain-specific wrapper key names (`users`, `pods`, `instances`) — but these are the hard cases anyway
- No structural heuristics — cannot check "count matches array length" or "pagination siblings present"
- `[...{...}]` accepts arrays of ANY objects, including `{"tags": ["a", "b"]}` if `tags` isn't in the attribute blocklist
- False positives: `{"items": [1, 2, 3]}` (array of primitives) would NOT match `[...{...}]` — good. But `{"data": [{"x": 1}], "name": "foo", "type": "widget", ...10 more fields}` (a resource with a `data` field) WOULD match — bad.

**False Positive Risk: MEDIUM-HIGH.** The schemas are too permissive because they cannot incorporate the heuristic signals (pagination siblings, count matching, dominant array, negative signals).

### Variant B3: CUE Schemas + Heuristic Scoring in Go (HYBRID)

Combine Variant B2's CUE schemas with a Go-side pre-filter or post-filter that applies the scoring heuristics from pudl-yqt.

**Design A — Pre-filter (TypePattern gates CUE matching):**
1. TypePattern `CollectionWrapperPattern` runs first with full heuristic scoring
2. If score ≥ threshold, sets `InferenceHints` to prefer `#CollectionWrapper*` schemas
3. CUE unification validates the structural match
4. If CUE fails, falls back normally

**Design B — Post-filter (CUE matches, Go validates):**
1. CUE unification matches one of the `#Wrapper*` schemas
2. Go post-filter applies heuristic scoring to the match
3. If score < threshold, demotes the match and retries without wrapper schemas
4. Returns wrapper match only if both CUE structure and heuristics agree

**Design C — TypePattern as primary detector, CUE for validation only:**
1. New `CollectionWrapperPattern` in typepattern/ with full scoring logic
2. When detected, generates or selects the appropriate CUE schema dynamically
3. CUE validates the match (structural correctness)
4. Import pipeline unwraps the collection

---

## 3. TypePattern Integration Analysis

The existing TypePattern system is well-suited for structural detection. Key evidence:

### GitLab CI Precedent

GitLab CI detection already uses structural analysis — iterating over fields, checking if values "look like" jobs:

```go
func detectGitLabCI(data map[string]interface{}) bool {
    for key, value := range data {
        if gitlabCIReservedKeys[key] { continue }
        if isGitLabJob(value) { jobCount++ }
    }
    return jobCount >= 1
}
```

This is directly analogous to "iterate over fields, check if any value is an array of homogeneous objects."

### TypePattern Confidence Scoring

The registry already calculates confidence scores (0.0–1.0) based on required fields, optional fields, and field values. The wrapper heuristics from pudl-yqt map cleanly onto this:

| pudl-yqt Heuristic | TypePattern Equivalent |
|---------------------|----------------------|
| Known wrapper key name (+0.35) | RequiredFields check (adapt: check if ANY field matches known names) |
| Pagination siblings (+0.25) | OptionalFields check (pagination keys as optional fields) |
| Count matches array length (+0.20) | Custom logic in TypeExtractor |
| Homogeneous array elements (+0.15) | Custom logic in TypeExtractor |
| Few top-level keys (+0.05) | Custom logic in TypeExtractor |
| Multiple similar arrays (−0.40) | Custom logic in TypeExtractor |
| Known attribute key (−0.30) | Custom logic in TypeExtractor |

### TypePattern Limitation

The current TypePattern interface uses `RequiredFields []string` — fields that MUST be present. For wrapper detection, no single field is always present; instead we need "at least one field from set X is present and is an array of objects." This requires extending the detection logic, likely via custom logic in the `TypeExtractor` function (which is already a `func(data map[string]interface{}) string` — arbitrary Go code).

---

## 4. Scoring Heuristic Integration with Schema-Based Detection

### Two-Phase Architecture

The most robust approach uses two phases:

**Phase 1 — Go-side structural detection (TypePattern):**
- Iterate over top-level fields
- Identify candidate array fields (arrays of objects with ≥1 element)
- Score each candidate using the pudl-yqt heuristics
- Return the best candidate's field name as the "type ID" (e.g., `wrapper:items`)

**Phase 2 — CUE schema validation:**
- Use the detected wrapper key to select the matching CUE schema (`#WrapperItems`)
- CUE unification validates structural correctness: the field IS an array of objects
- If no pre-defined schema exists for the key name, generate one dynamically or use a generic validator

### Scoring Algorithm (Refined from pudl-yqt)

```
For each top-level field where value is []interface{} with len ≥ 1:
  score = 0.0

  // Check array elements are objects (not primitives)
  if first element is NOT map[string]interface{}:
    skip this field

  // Strong signals
  if field_name in KNOWN_WRAPPER_KEYS:        score += 0.35
  if has_pagination_siblings(other_fields):    score += 0.25
  if count_field_matches_array_length:         score += 0.20
  if array_elements_are_homogeneous(≥80%):     score += 0.15

  // Weak signals
  if top_level_key_count <= 5:                 score += 0.05
  if array_byte_size > 50% of total:          score += 0.05

  // Negative signals
  if field_name in KNOWN_ATTRIBUTE_KEYS:       score -= 0.30
  if count_of_similar_sized_arrays >= 2:       score -= 0.40
  if non_pagination_scalar_fields > 6:         score -= 0.15

  // Domain-specific key names get a smaller bonus
  if field_name matches plural_noun_pattern:   score += 0.10

Best candidate = field with highest score
If best score >= 0.50: classify as collection wrapper
```

This scoring CANNOT be expressed in CUE — it requires imperative Go code with access to runtime values (array lengths, element types, field counts).

---

## 5. False Positive Risk Assessment

### Option A (Import-Time Unwrap) False Positive Risk

Option A applies the full scoring algorithm at import time in Go. False positive mitigation is straightforward: tune the threshold and heuristic weights based on test cases.

**Risk: LOW** — full heuristic power, adjustable threshold.

### Option B Pure (CUE Schemas Only) False Positive Risk

With Variant B2 (family of CUE schemas), the only constraint is "field X exists and is an array of objects." No pagination checking, no count matching, no attribute blocklist.

**Example false positives:**
- `{"data": [{"nested": "obj"}], "name": "config", "version": "1.0", ...}` — a resource with a `data` field
- `{"results": [{"passed": true}], "test_name": "unit-1", "suite": "core"}` — test result with `results` field
- `{"items": [{"key": "val"}], "menu_title": "File", "parent_id": 3}` — menu config with `items` field

**Risk: HIGH** — too many legitimate resources use `data`, `items`, `results` as field names.

### Option B Hybrid (CUE + TypePattern) False Positive Risk

TypePattern pre-filter applies full scoring before CUE matching. CUE validates structure. Both must agree.

**Mitigation strategies:**
1. Require score ≥ 0.50 from TypePattern (same as Option A)
2. CUE schema confirms array-of-objects structure (catches scoring errors)
3. Attribute blocklist prevents known resource field names
4. Multiple-array penalty prevents resources with several array attributes
5. Pagination signal requirement: if score is 0.35-0.49 (only wrapper key match), require at least one pagination sibling

**Risk: LOW** — equivalent to Option A, with CUE as an additional validation layer.

---

## 6. Concrete Recommendation

### Option A Is Strictly Better Than Pure Option B

Pure Option B (CUE schemas only) has unacceptable false positive risk because CUE cannot express the heuristic signals that distinguish wrappers from resources.

### Option B Hybrid ≈ Option A + CUE Schemas (No Significant Advantage)

The hybrid approach (TypePattern + CUE) achieves the same detection quality as Option A because all the intelligence is in the Go-side scoring. The CUE schemas add:
- A minor structural validation layer (array-of-objects check) — but Go already verifies this during scoring
- Schema infrastructure integration (priority, fallback chain) — useful but not essential
- User-extensible wrapper definitions via CUE — genuinely useful for custom API key names

### Final Recommendation: Option A with Optional CUE Schema Hooks

**Implement Option A (import-time detection + unwrap in Go) as the primary mechanism**, with an optional CUE schema layer for user extensibility:

1. **Primary detection**: `internal/importer/wrapper.go` — Go function with full heuristic scoring (as designed in pudl-yqt). This runs at import time, before schema inference.

2. **Optional CUE extensibility**: Allow users to define `#CollectionWrapper` schemas with specific key names in their schema packages. When a user-defined wrapper schema matches during inference, it triggers the same unwrap logic. This lets users declare `{"my_custom_items": [...]}` as a wrapper pattern without modifying Go code.

3. **Do NOT invest in a generic CUE `#CollectionWrapper`**: It's impossible in pure CUE and the family-of-schemas approach adds maintenance burden without improving detection quality.

### Why Not Full Hybrid?

The CUE schema layer in the hybrid approach:
- Adds ~15-30 boilerplate schema definitions
- Doesn't improve detection (Go scoring is strictly more powerful)
- Adds complexity to the inference pipeline (two detection paths)
- The user-extensibility benefit can be achieved with a simpler mechanism: a YAML/JSON config list of additional wrapper key names that the Go detector reads

**Bottom line: Option A is the right path. CUE's type system is not expressive enough for structural wrapper detection, and the TypePattern system (while capable of structural analysis) would just be reimplementing Option A's logic in a different location. Keep it simple — one detection path in `wrapper.go`.**

---

## Appendix A: CUE Schema Family (For Reference)

If the CUE extensibility path is pursued later, here's the schema template:

```cue
package wrapper

#WrapperItems: {
    _pudl: {
        schema_type:      "collection_wrapper"
        resource_type:    "generic.collection_wrapper"
        cascade_priority: 50
        identity_fields:  []
        tracked_fields:   ["items"]
        compliance_level: "permissive"
        wrapper_key:      "items"  // custom metadata for unwrap logic
    }
    items: [...{...}]
    ...
}
```

This could be templated for each key name, but the recommendation is to avoid this unless user extensibility becomes a priority.

## Appendix B: TypePattern Wrapper Detector Sketch

If a TypePattern approach were used (for reference — not recommended over Option A):

```go
var CollectionWrapperPattern = &TypePattern{
    Name:      "collection-wrapper",
    Ecosystem: "generic",
    Priority:  10, // Low priority — check after specific types
    // No RequiredFields — detection is fully custom
    TypeExtractor: func(data map[string]interface{}) string {
        best, score := detectWrapperField(data)
        if score >= 0.50 {
            return "wrapper:" + best
        }
        return ""
    },
}

func detectWrapperField(data map[string]interface{}) (string, float64) {
    var bestKey string
    var bestScore float64

    for key, val := range data {
        arr, ok := val.([]interface{})
        if !ok || len(arr) == 0 { continue }
        if _, ok := arr[0].(map[string]interface{}); !ok { continue }

        score := scoreWrapperCandidate(key, arr, data)
        if score > bestScore {
            bestKey, bestScore = key, score
        }
    }
    return bestKey, bestScore
}
```

This is functionally identical to Option A's `DetectCollectionWrapper()` — the only difference is where it lives (typepattern/ vs importer/).
