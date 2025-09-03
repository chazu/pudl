# PUDL Implementation Plan (Incremental)

This plan focuses on small, incremental steps toward a minimally usable tool. Each step should be completable in a few hours to a day of work.

## Phase 1: CLI Foundation & Basic Structure

### Step 1.1: Cobra CLI Migration ✅ Complete
**Goal**: Replace current simple CLI with Cobra structure
- [x] Create `cmd/` directory structure
- [x] Implement root command with global flags
- [x] Migrate current CUE processing to `pudl process <cue-file>` subcommand
- [x] Add basic help and version commands
- [x] Preserve existing functionality exactly

### Step 1.2: Directory Initialization ✅ Complete
**Goal**: Basic PUDL workspace setup
- [x] Implement `pudl init` command
- [x] Create `~/.pudl/` directory structure:
  - `~/.pudl/schema/` (git repository)
  - `~/.pudl/data/` (data storage)
  - `~/.pudl/config.yaml` (basic configuration)
- [x] Initialize git repository in schema directory
- [x] Add auto-initialization check on first run

### Step 1.3: Basic Configuration ✅ Complete
**Goal**: Simple configuration management
- [x] Define configuration structure (YAML)
- [x] Implement config loading/saving
- [x] Add `pudl config` command for viewing/editing config
- [x] Support basic settings (schema repo path, data path)
- [x] Add `pudl config set <key> <value>` for editing configuration
- [x] Add `pudl config reset` for restoring defaults
- [x] Add path validation and error handling

## Phase 2: Data Storage Foundation

### Step 2.1: Data Storage Architecture Discussion ✅ Complete
**Goal**: Clarify data storage approach before implementation
- [x] **DECIDED**: Date-based partitioning with future indexing support
- [x] **DECIDED**: CUE schema integration with package organization
- [x] **DECIDED**: Field-level schema evolution and resource change tracking
- [x] **DECIDED**: Zygomys rule engine for schema assignment (not generation)
- [x] **DECIDED**: Never reject data - mark as outliers or unknown/catchall
- [x] **DECIDED**: Timestamp + origin naming convention for raw data

**Architecture Summary**:
```
~/.pudl/data/
├── raw/YYYY/MM/DD/YYYYMMDD_HHMMSS_origin.ext
├── metadata/YYYYMMDD_HHMMSS_origin.ext.meta (JSON with CUE schema info)
├── catalog/inventory.json, schema_assignments.json, resource_tracking.json
└── schemas/ -> ~/.pudl/schema/ (CUE packages: aws/, k8s/, unknown/)
```

### Step 2.2: Basic Data Import ✅ Complete
**Goal**: Simple data ingestion with CUE schema integration
- [x] Implement `pudl import --path <file>` command
- [x] Add format detection (JSON, YAML, CSV basic detection)
- [x] Store raw data with timestamp + origin naming convention
- [x] Create metadata structure with CUE schema assignment
- [x] Implement basic rule-based schema assignment (simplified Zygomys for now)
- [x] Create data inventory and schema assignment catalog
- [x] Add catchall schema for unclassified data
- [x] Auto-create basic CUE schemas (AWS, K8s, unknown)

### Step 2.3: Data Retrieval ✅ Complete
**Goal**: Basic data access and listing
- [x] Implement `pudl list` command (show imported data)
- [x] Add `pudl show <data-id>` command (display specific data)
- [x] Advanced filtering by schema, origin, format
- [x] Sorting and limiting options (`--sort-by`, `--reverse`, `--limit`)
- [x] Verbose mode with summary statistics and file paths
- [x] Pretty-printed metadata and raw data display
- [x] Human-readable file size formatting

## Phase 3: Schema Management Basics

### Step 3.1: Schema Storage ✅ Complete
**Goal**: Basic schema file management
- [x] Define schema file naming conventions
- [x] Implement `pudl schema list` command
- [x] Add `pudl schema add <name> <cue-file>` command
- [x] Basic schema validation (CUE syntax check)

### Step 3.2: Schema-Data Association ✅ Complete
**Goal**: Manual schema assignment to data with cascading validation
- [x] Add `--schema <name>` flag to import command
- [x] Implement cascading validation with CUE schema inheritance
- [x] Store schema association metadata with validation results
- [x] Add comprehensive validation reporting with compliance status
- [x] Support policy-level schemas inheriting from base schemas
- [x] Complete cascade chain: policy → base → generic → catchall
- [x] **MAJOR**: Complete CUE schema inheritance and cross-reference support
- [x] **MAJOR**: Fix CUE hidden field access for metadata extraction
- [x] **MAJOR**: Implement proper CUE module loading with load package
- [x] **MAJOR**: Enable policy schemas inheriting from base schemas
- [x] **MAJOR**: Full compliance status reporting (COMPLIANT/NON-COMPLIANT/UNKNOWN)

### Step 3.3: Git Integration for Schemas ✅ Complete
**Goal**: Version control for schemas
- [x] Implement `pudl schema commit -m "message"` command
- [x] Add `pudl schema status` (show uncommitted changes)
- [x] Basic git operations wrapper (add, commit, log)
- [x] Schema change tracking
- [x] **MAJOR**: Complete git integration with CLI commands matching documented functionality

## Phase 3.5: Architecture Improvements (Critical for Phase 4/5)
**Goal**: Address architectural blockers identified in code review before proceeding

### Step 3.5.1: Error Handling Architecture Discussion ✅ **COMPLETE**
**Goal**: Plan error handling strategy before implementation
- [x] **DISCUSS**: Error handling patterns for CLI vs TUI compatibility
- [x] **DISCUSS**: Error code taxonomy and recovery strategies
- [x] **DISCUSS**: Progress reporting and cancellation mechanisms
- [x] **DISCUSS**: Error context preservation and user guidance

### Step 3.5.2: Error Handling Implementation ✅ **COMPLETE**
**Goal**: Replace log.Fatal() calls to enable Bubble Tea integration
- [x] Created `internal/errors` package with PUDLError types and constructors
- [x] Implemented context-aware error handlers (CLI, TUI, Test)
- [x] Updated internal packages (config, lister, git) to return structured errors
- [x] Converted CLI commands: import, list, show, config, init, process
- [x] Verified error handling with comprehensive testing
- [x] **UNBLOCKED**: Phase 5 Bubble Tea UI integration now possible
- [ ] **REMAINING**: Complete schema command conversions (list, status, commit, log)

### Step 3.5.3: Memory & Performance Architecture Discussion ⚠️ **HIGH PRIORITY**
**Goal**: Plan streaming and memory management strategy
- [ ] **DISCUSS**: Streaming parser architecture and chunk size strategies
- [ ] **DISCUSS**: Memory limit configuration and monitoring approach
- [ ] **DISCUSS**: Progress reporting interface design
- [ ] **DISCUSS**: Backward compatibility for existing data formats

### Step 3.5.4: Memory & Performance Foundation ⚠️ **HIGH PRIORITY**
**Goal**: Enable large file support and improve performance
- [ ] Implement streaming parsers for JSON/YAML/CSV (replace full-memory loading)
- [ ] Add progress reporting infrastructure for long operations
- [ ] Create configurable memory limits and chunk sizes
- [ ] **IMPACT**: Currently blocks handling of large datasets

### Step 3.5.5: Catalog Architecture Discussion ⚠️ **HIGH PRIORITY**
**Goal**: Plan catalog storage and indexing strategy
- [ ] **DISCUSS**: SQLite vs other storage backends (DuckDB, embedded options)
- [ ] **DISCUSS**: Index design for common query patterns
- [ ] **DISCUSS**: Migration strategy from JSON catalog to new format
- [ ] **DISCUSS**: Backup and recovery mechanisms for catalog data

### Step 3.5.6: Catalog Scalability Implementation ⚠️ **HIGH PRIORITY**
**Goal**: Replace linear search catalog with indexed system
- [ ] Design SQLite-based catalog with proper indexing
- [ ] Implement pagination for large result sets
- [ ] Add indexes for schema, origin, and timestamp queries
- [ ] Migrate existing JSON catalog to new format
- [ ] **IMPACT**: Current O(n) search won't scale beyond thousands of entries

## Phase 4: Basic Schema Inference

### Step 4.1: Rule Engine Architecture Discussion ⚠️ **ARCHITECTURE CHANGE**
**Goal**: Plan rule engine integration strategy before implementation
- [ ] **DISCUSS**: Zygomys embedding approach and performance implications
- [ ] **DISCUSS**: Rule file organization and loading mechanisms
- [ ] **DISCUSS**: Rule execution model and error handling strategies
- [ ] **DISCUSS**: Built-in vs user-defined rules and extensibility
- [ ] **DISCUSS**: Rule configuration format compatible with Zygomys
- [ ] **DISCUSS**: Migration strategy from hard-coded rules

### Step 4.2: Rule Engine Abstraction ⚠️ **ARCHITECTURE CHANGE**
**Goal**: Prepare for Zygomys integration with proper abstraction
- [ ] Create `RuleEngine` interface for pluggable rule systems
- [ ] Abstract current hard-coded rules into configurable format
- [ ] Design rule configuration format compatible with Zygomys
- [ ] Implement rule engine registry for runtime switching
- [ ] **REASON**: Current hard-coded rules block Zygomys integration

### Step 4.3: Zygomys Integration
**Goal**: Replace rule-based assignment with Zygomys rule engine
- [ ] Integrate Zygomys library through RuleEngine interface
- [ ] Migrate existing detection rules to Zygomys format
- [ ] Implement basic JSON→CUE schema inference rules
- [ ] Add `--infer-schema` flag to import command
- [ ] Store inferred schemas as "unconfirmed"

### Step 4.4: Schema Review Workflow
**Goal**: User confirmation of inferred schemas
- [ ] Implement `pudl schema review` command
- [ ] Show pending/unconfirmed schemas
- [ ] Add approve/reject/edit workflow
- [ ] Basic interactive prompts (before Bubble Tea)

## Phase 5: Enhanced Features

### Step 5.1: Bubble Tea Architecture Discussion
**Goal**: Plan interactive UI integration strategy
- [ ] **DISCUSS**: TUI architecture and state management approach
- [ ] **DISCUSS**: Command-line vs interactive mode coexistence
- [ ] **DISCUSS**: Progress reporting and cancellation in TUI context
- [ ] **DISCUSS**: Error handling and user feedback in interactive mode

### Step 5.2: Bubble Tea Integration
**Goal**: Improved interactive workflows
- [ ] Add Bubble Tea dependency
- [ ] Implement interactive schema review interface
- [ ] Enhanced data browsing interface
- [ ] Interactive import workflow

### Step 5.3: Outlier Detection Architecture Discussion
**Goal**: Plan policy compliance and outlier detection strategy
- [ ] **DISCUSS**: Two-tier schema architecture (base vs policy schemas)
- [ ] **DISCUSS**: Compliance scoring and threshold mechanisms
- [ ] **DISCUSS**: Outlier reporting and visualization approaches
- [ ] **DISCUSS**: Integration with existing validation pipeline

### Step 5.4: Basic Outlier Detection
**Goal**: Simple policy compliance checking
- [ ] Implement basic policy schema concept
- [ ] Add compliance checking during import
- [ ] Simple outlier reporting

### Step 5.5: Performance & Storage Architecture Discussion
**Goal**: Plan advanced storage and performance optimizations
- [ ] **DISCUSS**: Parquet integration approach and benefits
- [ ] **DISCUSS**: DuckDB integration strategy for analytics
- [ ] **DISCUSS**: Data lake organization and partitioning strategies
- [ ] **DISCUSS**: Query optimization and caching mechanisms

### Step 5.6: Performance & Storage Optimization
**Goal**: Handle larger datasets efficiently
- [ ] Implement efficient data storage format
- [ ] Add indexing for common queries
- [ ] Integrate advanced analytics capabilities

## Current State (100% Complete - Core Features)
- ✅ Basic CUE processing with custom functions
- ✅ Project structure and build system
- ✅ Cobra CLI Migration with preserved functionality
- ✅ Directory Initialization with auto-init and manual init
- ✅ Basic Configuration with editing capabilities
- ✅ Data Storage Architecture Discussion with CUE integration
- ✅ Basic Data Import with schema assignment and catalog
- ✅ Data Listing and Querying with filtering and detailed views
- ✅ **Step 3.1**: Schema Storage and Management with comprehensive validation
- ✅ **Step 3.2**: Schema-Data Association with complete CUE inheritance and cascading validation
- ✅ **Step 3.3**: Git Integration for Schemas with full CLI command support

## Implementation Status
- ✅ **Git Integration**: Complete with `pudl schema commit/status/log` commands
- ✅ **Error Handling**: Structured error handling implemented, Bubble Tea integration unblocked
- 🚨 **Memory Usage**: Full-file loading prevents large dataset support (Phase 3.5.4)
- 🚨 **Catalog Performance**: Linear search won't scale (Phase 3.5.6)
- 🚨 **Rule Engine**: Hard-coded rules block Zygomys integration (Phase 4.2)
- ⚠️ **CUE Error Parsing**: Generic error messages instead of precise CUE validation details
- ⚠️ **CSV Schema Inference**: Basic CSV support without proper type detection
- ⚠️ **Metadata Extraction**: Only `_pudl` metadata extracted, missing legacy metadata

## Critical Path (Based on Code Review)
**Phase 3.5 partially complete - remaining items before Phase 4/5**
- ✅ **Phase 3.5.1-2**: Error handling implemented (Phase 5 unblocked)
- 🚨 **Phase 3.5.3-4**: Memory optimization (enables large datasets)
- 🚨 **Phase 3.5.5-6**: Catalog scalability (enables performance)
- 🚨 **Phase 4.1-2**: Rule engine abstraction (enables Zygomys)

## Next Priority (Quality Improvements)
- 🔄 **QUALITY**: Enhanced CUE error parsing for better user experience
- 🔄 **ROBUSTNESS**: Complete metadata extraction and error recovery
- 🔄 **PERFORMANCE**: CSV schema inference and memory optimization

## Success Criteria for Each Phase
- **Phase 1**: ✅ Can initialize PUDL workspace, import data, and manage basic configuration
- **Phase 2**: ✅ Can store and retrieve data with metadata, basic format detection works
- **Phase 3**: ✅ Can manually assign schemas to data, validate, and manage schema versions with git
- **Phase 3.5**: Can handle large files, graceful errors, and scalable catalog operations
- **Phase 4**: Can automatically infer basic schemas and review them with Zygomys
- **Phase 5**: Interactive workflows and basic outlier detection with Bubble Tea UI

## Testing Approach
- Unit tests for each component with mock data
- Integration tests using generated test data (not committed)
- Performance benchmarks for large datasets (Phase 3.5.4)
- ✅ Error handling tests for all failure scenarios (Phase 3.5.2)
- Avoid test data files in repository
- Focus on testing logic, not data formats

## Code Review Findings (2025-09-03)
**Comprehensive review identified critical architectural blockers for Phase 4/5:**
- ✅ **Error Handling**: log.Fatal() incompatible with Bubble Tea TUI → **RESOLVED**
- 🚨 **Memory Usage**: Full-file loading prevents large dataset support
- 🚨 **Catalog Performance**: Linear search O(n) won't scale beyond thousands of entries
- 🚨 **Rule Engine**: Hard-coded rules require complete rewrite for Zygomys

**See review.md for detailed analysis and recommendations**

## Error Handling Migration Progress (2025-09-03)
**Successfully implemented unified error handling architecture:**

### ✅ **Core Infrastructure Complete**
- Created `internal/errors` package with PUDLError types and 17 error codes
- Implemented context-aware handlers: CLIErrorHandler, TUIErrorHandler, TestErrorHandler
- Added error constructors with suggestions and recovery information
- Proper Go error interface with Error(), Unwrap(), Is() methods

### ✅ **Internal Packages Updated**
- `internal/config`: All functions return structured PUDLError types
- `internal/lister`: Updated catalog loading and entry finding with helpful errors
- `internal/git`: Repository operations with actionable error messages
- Pattern established for remaining packages (importer, validator, schema)

### ✅ **CLI Commands Converted**
- `cmd/import.go`: Complete conversion with file validation and config errors
- `cmd/list.go`: Complete conversion with catalog and filter errors
- `cmd/show.go`: Complete conversion with entry lookup errors
- `cmd/config.go`: All subcommands (show, set, reset) converted
- `cmd/init.go`: Complete conversion with workspace initialization errors
- `cmd/process.go`: Complete conversion with file format validation

### 🔄 **Remaining Work**
- `cmd/schema.go`: Schema add command converted, remaining commands (list, status, commit, log) still use log.Fatal()
- Additional internal packages can be updated as needed using established pattern

### ✅ **Verification Complete**
- Tested error scenarios: file not found, invalid config, unsupported formats
- Verified proper exit codes (2 for invalid usage, 1 for general errors)
- Confirmed user-friendly error messages with actionable suggestions
- **Phase 5 Bubble Tea integration now unblocked**