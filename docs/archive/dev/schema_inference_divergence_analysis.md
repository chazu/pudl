# PUDL Schema Inference Divergence Analysis

## Executive Summary

This analysis identifies all locations in the PUDL codebase where **hard-coded Go logic** is used for data classification and schema inference instead of the originally intended **dynamic CUE-based schema repository approach**. The system was designed to read schema definitions from CUE files in the user's PUDL schema repository, but multiple components have fallen back to hard-coded patterns and rules.

## Architecture Overview

### Intended Design (CUE-Based)
- **Dynamic Schema Loading**: Read schema definitions from `~/.pudl/schema/` CUE files
- **CUE Module System**: Use proper CUE modules with cross-references and third-party dependencies
- **Metadata-Driven**: Extract field patterns, validation rules, and cascade priorities from CUE schema metadata
- **User-Extensible**: Users can add custom schemas via `pudl schema add` command

### Current Implementation (Hybrid)
- **Mixed Approach**: Some components use CUE loading, others use hard-coded Go logic
- **Hard-coded Patterns**: Multiple locations with embedded AWS/K8s detection rules
- **Incomplete CUE Integration**: CUE schemas are loaded but not fully utilized for pattern extraction

## Critical Divergence Points

### 1. Primary Schema Assignment Logic

**Location**: `internal/importer/schema.go` (Lines 11-131)
**Current Approach**: Hard-coded Go logic with embedded AWS/K8s patterns
**Should Be**: Dynamic CUE schema-based detection

```go
// HARD-CODED: AWS EC2 Instance detection
if i.hasFields(dataMap, []string{"InstanceId", "State", "InstanceType"}) {
    return "aws.#EC2Instance", 0.95
}

// HARD-CODED: Kubernetes resource detection
if i.hasFields(dataMap, []string{"kind", "apiVersion", "metadata"}) {
    switch kind {
    case "Pod": return "k8s.#Pod", 0.95
    case "Service": return "k8s.#Service", 0.95
    // ... 10+ more hard-coded cases
    }
}
```

**Impact**: This is the **primary entry point** for schema assignment during data import. All data classification flows through this hard-coded logic instead of reading patterns from CUE schemas.

### 2. Streaming Schema Detection

**Location**: `internal/streaming/schema_detector.go` (Lines 291-350)
**Current Approach**: Hard-coded schema patterns in Go structs
**Should Be**: Patterns extracted from CUE schema definitions

```go
// HARD-CODED: AWS EC2 Instance pattern
d.patterns = append(d.patterns, SchemaPattern{
    Name: "aws.ec2-instance",
    Fields: []FieldPattern{
        {Name: "InstanceId", Type: "string", Required: true},
        {Name: "State", Type: "object", Required: true},
    },
    Optional: []FieldPattern{
        {Name: "InstanceType", Type: "string"},
        // ... more hard-coded fields
    },
})
```

**Impact**: Streaming data processing uses completely separate hard-coded patterns instead of leveraging the CUE schema system.

### 4. CUE Integration Incomplete Implementation

**Location**: `internal/streaming/cue_integration.go` (Lines 210-226)
**Current Approach**: Placeholder CUE pattern creation with TODO comments
**Should Be**: Full CUE schema introspection and pattern extraction

```go
// TODO: Extract from CUE - Currently creates empty patterns
pattern := SchemaPattern{
    Name:        fmt.Sprintf("%s.%s", cueSchema.Package, cueSchema.Name),
    Description: fmt.Sprintf("CUE schema: %s", cueSchema.Name),
    Fields:      []FieldPattern{}, // TODO: Extract from CUE
    Optional:    []FieldPattern{}, // TODO: Extract from CUE
}
```

**Impact**: The CUE integration exists but doesn't actually extract field patterns from CUE schemas, making it non-functional.

## Data Flow Analysis

### Current Schema Assignment Flow

1. **Data Import** → `internal/importer/importer.go`
2. **Schema Detection** → `internal/importer/schema.go` (hard-coded patterns)
3. **Schema Assignment** → Returns hard-coded schema names

### Intended Schema Assignment Flow

1. **Data Import** → `internal/importer/importer.go`
2. **CUE Schema Loader** → `internal/validator/cue_loader.go`
3. **Dynamic Pattern Extraction** → Extract patterns from CUE schema metadata
4. **Schema Assignment** → Return schema based on CUE-defined patterns

## Detailed Inventory of Hard-Coded Logic

### A. Primary Schema Detection (Critical)

| File | Lines | Hard-Coded Patterns | Impact |
|------|-------|-------------------|---------|
| `internal/importer/schema.go` | 34-131 | AWS EC2, S3, K8s Pod/Service/Deployment, etc. | **HIGH** - Primary import path |
| `internal/streaming/schema_detector.go` | 291-350 | AWS EC2, K8s Pod, S3 patterns | **MEDIUM** - Streaming processing |

### B. Pattern Definitions (Medium Priority)

| File | Lines | Hard-Coded Elements | Impact |
|------|-------|-------------------|---------|
| `internal/streaming/schema_detector.go` | 18-34 | `SchemaPattern` and `FieldPattern` structs | **MEDIUM** - Pattern structure |
| `internal/importer/cue_schemas.go` | 24-156 | CatchAll and Collection schemas | **LOW** - Fallback schemas |

### C. Detection Utilities (Cleaned Up)

| File | Lines | Status | Impact |
|------|-------|--------|---------|
| `internal/importer/detection.go` | 111-125 | ✅ **CLEANED** - Now uses filename only | **LOW** - Simple origin detection |
| `internal/streaming/cue_integration.go` | 154-180 | Schema name matching patterns | **LOW** - Name resolution |

**Note**: As of 2026-01-29, the `detectOrigin()` function has been simplified to just return the filename without extension. Hardcoded AWS/K8s pattern matching has been removed.

## Working CUE Infrastructure

### Functional Components

1. **CUE Module Loader** (`internal/validator/cue_loader.go`)
   - ✅ Loads CUE modules from schema directory
   - ✅ Extracts schema definitions and metadata
   - ✅ Handles cross-references and dependencies

2. **Schema Manager** (`internal/schema/manager.go`)
   - ✅ File-based schema operations
   - ✅ Package organization
   - ✅ Schema validation

3. **Cascade Validator** (`internal/validator/cascade_validator.go`)
   - ✅ CUE-based data validation
   - ✅ Schema cascade chains
   - ✅ Metadata-driven validation

### Missing Integration

The **critical gap** is that while CUE schemas are loaded and can validate data, they are **not used for initial schema detection/classification**. The detection logic still relies on hard-coded Go patterns.

## Migration Strategy Recommendations

### Phase 1: Core Infrastructure (High Priority)

1. **Extend CUE Schema Metadata**
   - Add detection patterns to CUE schema `_pudl` metadata
   - Define field patterns, required fields, optional fields
   - Include confidence scoring rules

2. **Create CUE Pattern Extractor**
   - Build component to extract detection patterns from CUE schemas
   - Convert CUE metadata to `SchemaPattern` structs
   - Handle pattern inheritance and fallbacks

3. **Replace Primary Detection Logic**
   - Modify `internal/importer/schema.go` to use CUE-extracted patterns
   - Replace hard-coded logic with dynamic pattern matching
   - Maintain backward compatibility during transition

### Phase 2: Streaming Integration (Medium Priority)

1. **Streaming Pattern Loading**
   - Update `internal/streaming/schema_detector.go` to use CUE patterns
   - Remove hard-coded pattern definitions
   - Enable dynamic pattern loading from CUE schemas

### Phase 3: Advanced Features (Low Priority)

1. **Runtime Schema Updates**
   - Support hot-reloading of schema changes
   - Dynamic pattern cache invalidation
   - Live schema repository monitoring

2. **User Experience Improvements**
   - Enhanced schema debugging tools
   - Pattern conflict detection
   - Schema coverage analysis

## Risk Assessment

### High Risk Areas

1. **Breaking Changes**: Modifying core detection logic could affect existing data classifications
2. **Performance Impact**: Dynamic CUE pattern extraction may be slower than hard-coded logic
3. **Complexity**: CUE schema introspection is more complex than static patterns

### Mitigation Strategies

1. **Gradual Migration**: Implement feature flags to switch between hard-coded and CUE-based detection
2. **Comprehensive Testing**: Create test suite comparing old vs. new detection results
3. **Performance Monitoring**: Benchmark CUE-based detection against current implementation
4. **Fallback Mechanisms**: Maintain hard-coded patterns as fallback for critical schemas

## Conclusion

The PUDL system has a **well-designed CUE infrastructure** that is currently **underutilized**. The primary issue is that schema detection/classification logic uses hard-coded Go patterns instead of reading from the user's CUE schema repository.

**Key Findings:**
- 🔴 **2 critical locations** with hard-coded schema detection logic (`internal/importer/schema.go`, `internal/streaming/schema_detector.go`)
- 🟡 **Incomplete CUE integration** - framework exists but patterns not extracted
- 🟢 **Functional CUE infrastructure** ready for integration (loader, validator)
- ✅ **Detection utilities cleaned** - `detectOrigin()` simplified (2026-01-29)

**Recommended Action:**
Focus on **Phase 1** migration to replace the core detection logic in `internal/importer/schema.go` with CUE schema-driven pattern extraction. This will restore the system to its intended design and enable users to fully customize schema detection through their CUE schema repository.