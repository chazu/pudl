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

### Step 1.2: Directory Initialization
**Goal**: Basic PUDL workspace setup
- [ ] Implement `pudl init` command
- [ ] Create `~/.pudl/` directory structure:
  - `~/.pudl/schema/` (git repository)
  - `~/.pudl/data/` (data storage)
  - `~/.pudl/config.yaml` (basic configuration)
- [ ] Initialize git repository in schema directory
- [ ] Add auto-initialization check on first run

### Step 1.3: Basic Configuration
**Goal**: Simple configuration management
- [ ] Define configuration structure (YAML)
- [ ] Implement config loading/saving
- [ ] Add `pudl config` command for viewing/editing config
- [ ] Support basic settings (schema repo path, data path)

## Phase 2: Data Storage Foundation

### Step 2.1: Data Storage Architecture Discussion
**Goal**: Clarify data storage approach before implementation
- [ ] **DISCUSS**: Directory structure for data storage
- [ ] **DISCUSS**: Metadata format for imported data
- [ ] **DISCUSS**: File naming conventions and partitioning strategy
- [ ] **DISCUSS**: Raw vs processed data organization

### Step 2.2: Basic Data Import
**Goal**: Simple data ingestion without schema inference
- [ ] Implement `pudl import --path <file>` command
- [ ] Add format detection (JSON, YAML, CSV basic detection)
- [ ] Store raw data with basic metadata (timestamp, source, format)
- [ ] Create simple data inventory (list what's been imported)

### Step 2.3: Data Retrieval
**Goal**: Basic data access and listing
- [ ] Implement `pudl list` command (show imported data)
- [ ] Add `pudl show <data-id>` command (display specific data)
- [ ] Basic filtering by date, format, source
- [ ] Simple data export functionality

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
- 🔄 **NEXT**: Step 1.2 - Directory Initialization

## Success Criteria for Each Phase
- **Phase 1**: Can initialize PUDL workspace, import data, and manage basic configuration
- **Phase 2**: Can store and retrieve data with metadata, basic format detection works
- **Phase 3**: Can manually assign schemas to data and validate
- **Phase 4**: Can automatically infer basic schemas and review them
- **Phase 5**: Interactive workflows and basic outlier detection

## Testing Approach
- Unit tests for each component with mock data
- Integration tests using generated test data (not committed)
- Avoid test data files in repository
- Focus on testing logic, not data formats