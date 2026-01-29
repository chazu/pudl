# PUDL - Development Plan

## Project Overview

PUDL (Personal Unified Data Lake) is a CLI tool for SRE/platform engineers to manage and analyze cloud infrastructure data. It creates a personal data lake for cloud resources, Kubernetes objects, logs, and metrics using CUE-based schema validation.

## Current State

### Core Functionality (Implemented)
- **Data Import** - Supports JSON, YAML, CSV, and NDJSON with format detection
- **Streaming Support** - Process large files using Content-Defined Chunking (CDC)
- **SQLite Catalog** - High-performance catalog database
- **CUE Schema Management** - Schema loading, validation, and git version control
- **Bootstrap Schemas** - Base schemas (catchall, collections) embedded and copied on init
- **User Repository** - Data lake at `~/.pudl/` with schema/, data/, config.yaml

### Schema Infrastructure
- `internal/importer/bootstrap/` - Embedded bootstrap CUE schemas
- `internal/schema/manager.go` - Schema loading and management
- `internal/validator/` - CUE validation and cascade validation
- `internal/streaming/cue_integration.go` - CUE-based schema detection (framework)

### What Works
- `pudl init` - Initializes user repository with CUE module and bootstrap schemas
- `pudl import` - Imports data files to the data lake
- `pudl schema list` - Lists available schemas
- `pudl validate` - Validates data against schemas
- Format detection (JSON, YAML, CSV, NDJSON)
- Basic schema inference framework

## Technical Debt Addressed (2026-01-29 Cleanup)

### Completed
- [x] Removed `internal/importer/cue.mod/` - Was incorrectly creating CUE module in project repo
- [x] Removed `internal/importer/pudl/` - Duplicate of bootstrap/pudl directory
- [x] Consolidated CUE module creation - Removed duplicate `createCUEModule()` from cue_schemas.go
- [x] Simplified `detectOrigin()` - Removed hardcoded AWS/K8s pattern matching, now uses filename only
- [x] Updated tests to reflect simplified detection

### Design Decisions Made
- **No rules package** - The previously planned `internal/rules/` with zygomys Lisp support is not being implemented
- **Schema inference strategy TBD** - Strategic decisions about CUE-based schema inference deferred for later
- **Origin detection simplified** - Origin is now just filename; schema matching should be handled by CUE patterns

## Future Development (Prioritized)

### High Priority
1. **Complete CUE-based schema detection** - The `internal/streaming/cue_integration.go` has placeholder code that needs implementation
2. **Improve error messages** - Better user-facing error messages for common issues
3. **Schema review workflow** - Complete the TUI-based schema review flow

### Medium Priority
1. **Schema pattern extraction** - Extract detection patterns from CUE schema `_pudl` metadata
2. **User-defined schemas** - Support custom schemas in user's `~/.pudl/schema/`
3. **Collection support** - Improve NDJSON/collection handling

### Low Priority
1. **Hot-reloading** - Reload schemas without restart
2. **Schema debugging tools** - Tools to help develop and test schemas
3. **Pattern conflict detection** - Detect conflicting schema patterns

## Architecture Notes

### User Repository (`~/.pudl/`)
```
~/.pudl/
├── config.yaml           # Configuration file
├── data/                  # Imported data files
├── schema/
│   ├── cue.mod/
│   │   └── module.cue    # CUE module with k8s deps
│   ├── pudl/
│   │   ├── unknown/
│   │   │   └── catchall.cue
│   │   └── collections/
│   │       └── collections.cue
│   └── examples/         # Usage examples
└── catalog.db            # SQLite catalog
```

### Bootstrap Flow
1. `pudl init` calls `internal/init/initCUEModule()` to create `cue.mod/module.cue`
2. `pudl init` calls `importer.CopyBootstrapSchemas()` to copy embedded schemas
3. `cue mod tidy` is run to fetch k8s dependencies (if cue is available)

### Import Flow
1. `ensureBasicSchemas()` verifies schema repo is initialized (errors if not)
2. `detectFormat()` determines file format
3. `detectOrigin()` returns filename without extension
4. Schema inference assigns appropriate schema (currently simplified)
5. Data stored in catalog with metadata

## Files Reference

### Core Packages
- `internal/importer/` - Data import logic
- `internal/schema/` - Schema management
- `internal/validator/` - CUE validation
- `internal/streaming/` - Large file processing
- `internal/init/` - Repository initialization
- `internal/database/` - SQLite catalog

### Configuration
- `internal/config/config.go` - Configuration loading
- `cmd/*.go` - CLI commands
