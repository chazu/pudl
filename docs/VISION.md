# PUDL Vision & Architecture

This document describes what PUDL is today and where it's headed.

## Core Purpose

PUDL is a **personal infrastructure knowledge base and data lake** designed to help SRE/platform engineers/software engineers navigate, learn about, and isolate issues in their cloud environments. The long-term goal is **outlier detection and sprawl reduction** in cloud infrastructure and configuration.

## What Exists Today

### Data Lake Structure
- **Schema Repository**: `~/.pudl/schema/` - Git repository containing CUE schema definitions
- **Data Storage**: `~/.pudl/data/` - Imported data files with full provenance metadata
- **Catalog Database**: SQLite-based catalog with query, filter, and pagination support
- **Self-contained**: No external dependencies required for PUDL binary operation

### Schema System
- **CUE Lang**: Schema definition, validation, and constraint checking
- **Cascade Validation**: Multi-level schema matching (specific -> fallback -> catchall)
- **Schema Inference**: Automatic CUE-based schema detection using heuristics and CUE unification
- **Schema Generation**: `pudl schema new` generates CUE schemas from imported data
- **Schema Name Normalization**: Canonical format for consistent schema references
- **Git Integration**: `pudl schema status/commit/log` for version-controlled schemas

### Data Management
- **Multi-format Import**: JSON, YAML, CSV, NDJSON with automatic format detection
- **Collection Support**: Collections split into individual items with parent references
- **Provenance Tracking**: Timestamp, origin, format, and schema assignment tracked per entry
- **Export**: Multi-format output support

### CLI Commands
- `pudl init` - Initialize the data lake
- `pudl import` - Import data files
- `pudl list` - Query and filter catalog entries
- `pudl export` - Export data in various formats
- `pudl delete` - Remove catalog entries
- `pudl validate` - Validate data against schemas
- `pudl schema *` - Full schema lifecycle (list, add, new, show, edit, reinfer, migrate, status, commit, log)
- `pudl doctor` - Health check utility

### Technology Stack
- **Go**: Core application with Cobra CLI framework
- **CUE Lang**: Schema definition and validation
- **SQLite**: Catalog database

## Future Vision

The following features are aspirational and not yet implemented.

### Phase 1: Analytical Layer (Next Priority)
- **Diff**: Compare two imports of the same resource type, show what changed
- **Summary/Stats**: Aggregate views ("47 EC2 instances, 3 outliers")
- **Basic Outlier Detection**: Given N instances of a schema, identify unusual field values
- These features transform PUDL from "a place data goes" into "a tool that tells me things"

### Phase 2: Schema Intelligence
- **Two-Tier Schema System**: Broad type recognition + policy compliance
  - Nothing rejected if it's a valid instance of the resource type
  - Easy identification of policy violations/outliers
  - Enables infrastructure standardization efforts
- **Schema Drift Detection**: "This resource used to validate, now it doesn't"
- **Schema Coverage Reports**: "37% of data matches a specific schema, 63% is generic"

### Phase 3: Correlation & Cross-Source
- **Cross-Source Correlation**: Link AWS resources to K8s resources
- **Temporal Tracking**: Same resource across multiple imports
- **Resource Identity**: Determine what constitutes a "change" vs "different resource"

### Phase 4: Advanced Analytics
- **DuckDB/Parquet Integration**: Analytical query engine for large datasets
- **Expert System Components**: Automatic detection of common substructures
- **Dashboard/Reporting Interfaces**: Visual representation of infrastructure state

## Implementation Philosophy

- **Small incremental steps** to avoid large rework cycles
- **Keep doors open** for future capabilities without over-engineering
- **User-friendly workflows** with clear CLI output
- **Minimal viable features** before expanding scope

## Open Questions

1. **Schema Drift Handling**: Hard validation vs. soft warnings vs. automatic evolution
2. **Correlation Timing**: Ingestion-time vs. on-demand computation
3. **Data Partitioning Strategy**: Optimal partitioning for analytical queries
4. **Identity Determination**: How to identify what constitutes a "change" vs "different resource"
