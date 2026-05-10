# Type Detection Integration into Import Flow

**Task ID**: pudl-n1w  
**Date**: 2026-02-13

## Summary

Integrated the type detection system into the main import flow so that when schema inference fails or gives low confidence, the importer automatically detects data types and generates appropriate schemas.

## Changes Made

### Modified Files

- `internal/importer/importer.go`

### Implementation Details

1. **Added new fields to Importer struct**:
   - `typeRegistry *typepattern.Registry` - Registry of type patterns for detection
   - `schemaGen *schemagen.Generator` - Generator for creating schemas

2. **Initialize in New function**:
   - Create registry with `typepattern.NewRegistry()`
   - Register Kubernetes patterns with `typepattern.RegisterKubernetesPatterns()`
   - Create schema generator with `schemagen.NewGenerator(schemaPath)`

3. **Added `handleUnmatchedData` method**:
   - Triggers when inference confidence < 0.5 OR schema is catchall
   - Uses `typeRegistry.Detect()` to identify data type
   - Uses `schemaGen.GenerateFromDetectedType()` to create schema
   - Uses `schemaGen.WriteSchema()` to save schema (without overwrite)
   - Uses `inferrer.Reload()` to pick up new schema
   - Re-runs inference to match the new schema

4. **Added `isCatchall` helper**:
   - Uses `schemaname.IsFallbackSchema()` to check if schema is generic fallback

5. **Integrated into existing flows**:
   - `assignItemSchema`: For collection items
   - `ImportFile`: For single file imports

## Public API

No public API changes. The type detection is transparent and automatic.

## Testing

All existing tests pass. The `TestImportFile_KubernetesDetection` test shows the type detection working:
```
Detected [kubernetes] resource: v1:Pod
```

