# Schema Generate-Type Command

## Summary

Added CLI command `pudl schema generate-type` for manually generating schemas from detected types in the type registry.

## Changes Made

### New Files
- `cmd/schema_generate_type.go` - New command implementation

### Modified Files
- `internal/typepattern/kubernetes.go` - Added helper functions
- `internal/schemagen/generator.go` - Added syntax-only validation for import-based schemas

## Public API

### Command: `pudl schema generate-type`

Generates a CUE schema for a known type from the type registry without needing sample data.

**Kubernetes types:**
```bash
pudl schema generate-type --kind Job --api-version batch/v1
pudl schema generate-type --kind Deployment --api-version apps/v1
```

**Flags:**
- `--kind` - Kubernetes resource kind (e.g., Job, Deployment)
- `--api-version` - Kubernetes API version (e.g., batch/v1, apps/v1)
- `--ecosystem` - Ecosystem name (defaults to kubernetes)
- `--type` - Generic type identifier for non-K8s types
- `--force` - Overwrite existing schema
- `--dry-run` - Print generated schema without saving

### New Functions

#### typepattern package
- `BuildKubernetesDetectedType(kind, apiVersion string) *DetectedType` - Build DetectedType from CLI flags
- `GetKnownKubernetesTypes() []string` - List all known Kubernetes types

#### schemagen package
- `ValidateCUESyntax(content string) error` - Syntax-only CUE validation (no import resolution)
- `WriteSchemaWithSyntaxCheck(result, content, force) error` - Write schema with syntax-only validation

## Technical Details

Import-based schemas (which extend canonical types like `cue.dev/x/k8s.io/...`) cannot be fully validated until dependencies are fetched via `cue mod tidy`. The new `WriteSchemaWithSyntaxCheck` method allows writing these schemas with syntax-only validation, deferring semantic validation until after dependencies are available.

