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

### Step 3.1: Schema Storage
**Goal**: Basic schema file management
- [ ] Define schema file naming conventions
- [ ] Implement `pudl schema list` command
- [ ] Add `pudl schema add <name> <cue-file>` command
- [ ] Basic schema validation (CUE syntax check)

### Step 3.2: Schema-Data Association
**Goal**: Manual schema assignment to data
- [ ] Add `--schema <name>` flag to import command
- [ ] Implement basic data validation against schema
- [ ] Store schema association metadata with data
- [ ] Add validation reporting (pass/fail with basic error info)

### Step 3.3: Git Integration for Schemas
**Goal**: Version control for schemas
- [ ] Implement `pudl schema commit -m "message"` command
- [ ] Add `pudl schema status` (show uncommitted changes)
- [ ] Basic git operations wrapper (add, commit, log)
- [ ] Schema change tracking

## Phase 4: Basic Schema Inference

### Step 4.1: Zygomys Integration Architecture Discussion
**Goal**: Plan rule engine integration before implementation
- [ ] **DISCUSS**: Zygomys embedding approach
- [ ] **DISCUSS**: Rule file organization and loading
- [ ] **DISCUSS**: Rule execution model and error handling
- [ ] **DISCUSS**: Built-in vs user-defined rules

### Step 4.2: Simple Schema Inference
**Goal**: Basic automatic schema generation
- [ ] Integrate Zygomys library
- [ ] Implement basic JSON→CUE schema inference rules
- [ ] Add `--infer-schema` flag to import command
- [ ] Generate simple CUE schemas from data structure
- [ ] Store inferred schemas as "unconfirmed"

### Step 4.3: Schema Review Workflow
**Goal**: User confirmation of inferred schemas
- [ ] Implement `pudl schema review` command
- [ ] Show pending/unconfirmed schemas
- [ ] Add approve/reject/edit workflow
- [ ] Basic interactive prompts (before Bubble Tea)

## Phase 5: Enhanced Features

### Step 5.1: Bubble Tea Integration
**Goal**: Improved interactive workflows
- [ ] Add Bubble Tea dependency
- [ ] Implement interactive schema review interface
- [ ] Enhanced data browsing interface
- [ ] Interactive import workflow

### Step 5.2: Basic Outlier Detection
**Goal**: Simple policy compliance checking
- [ ] **DISCUSS**: Two-tier schema architecture
- [ ] Implement basic policy schema concept
- [ ] Add compliance checking during import
- [ ] Simple outlier reporting

### Step 5.3: Performance & Storage Optimization
**Goal**: Handle larger datasets efficiently
- [ ] **DISCUSS**: Parquet integration approach
- [ ] **DISCUSS**: DuckDB integration strategy
- [ ] Implement efficient data storage format
- [ ] Add indexing for common queries

## Current State
- ✅ Basic CUE processing with custom functions
- ✅ Project structure and build system
- ✅ Cobra CLI Migration with preserved functionality
- ✅ Directory Initialization with auto-init and manual init
- ✅ Basic Configuration with editing capabilities
- ✅ Data Storage Architecture Discussion with CUE integration
- ✅ Basic Data Import with schema assignment and catalog
- ✅ Data Listing and Querying with filtering and detailed views
- 🔄 **NEXT**: Step 3.1 - Schema Storage and Management

## Success Criteria for Each Phase
- **Phase 1**: ✅ Can initialize PUDL workspace, import data, and manage basic configuration
- **Phase 2**: ✅ Can store and retrieve data with metadata, basic format detection works
- **Phase 3**: Can manually assign schemas to data and validate
- **Phase 4**: Can automatically infer basic schemas and review them
- **Phase 5**: Interactive workflows and basic outlier detection

## Testing Approach
- Unit tests for each component with mock data
- Integration tests using generated test data (not committed)
- Avoid test data files in repository
- Focus on testing logic, not data formats