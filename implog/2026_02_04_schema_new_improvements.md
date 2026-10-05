# Schema New Command Improvements

**Date:** 2026-02-04

## Summary

Enhanced `pudl schema new` command for better agent usability with three improvements:
1. Added `--force` flag to overwrite existing schema files
2. Added JSON output support via `--json` flag
3. Improved error messages when schemas already exist

Also consolidated default schemas by renaming `#CatchAll` to `#Item` and removing the redundant `#CollectionItem` schema.

## Public API

### New Command Flag
```bash
pudl schema new --force  # Overwrite existing schema files
```

### JSON Output
```bash
pudl schema new --from <id> --path <path> --json
```

Returns:
```json
{
  "success": true,
  "file_path": "/path/to/schema.cue",
  "package_name": "ec2",
  "definition_name": "Instance",
  "field_count": 11,
  "inferred_identity_fields": ["ImageId", "InstanceId"],
  "is_collection": false
}
```

For collections:
```json
{
  "success": true,
  "file_path": "/path/to/collection.cue",
  "is_collection": true,
  "new_item_schemas": [{"file_path": "...", "definition_name": "...", "field_count": 12}],
  "existing_schema_refs": ["pudl.schemas/aws/s3@v0:#Bucket"]
}
```

### New Types

**SchemaExistsError** (`internal/schemagen/generator.go`)
- Custom error type for when schema file already exists
- Contains `FilePath`, `DefinitionName`, `PackagePath`

**SchemaNewOutput** (`internal/ui/output.go`)
- Structured output for schema new command

**SchemaNewItemOutput** (`internal/ui/output.go`)
- Item schema info within collection generation output

### Schema Changes

The core schema package (`pudl/core`) now contains:
- `#Item` - Universal fallback schema for any individual piece of data (replaces `#CatchAll` and `#CollectionItem`)
- `#Collection` - Multi-record file collections (NDJSON, etc.)

## Files Changed

- `cmd/schema.go` - Added flag, JSON output, improved error handling
- `internal/schemagen/generator.go` - Added SchemaExistsError, updated WriteSchema signature
- `internal/ui/output.go` - Added output structs
- `internal/importer/bootstrap/pudl/core/core.cue` - Renamed #CatchAll to #Item
- Various test files updated for #Item rename

## Tests Added

- `internal/schemagen/generator_test.go` - Tests for WriteSchema and SchemaExistsError
- `internal/ui/output_test.go` - Tests for SchemaNewOutput JSON serialization

