# PUDL Vision & Architecture Clarifications

This document captures the detailed vision and architectural decisions for PUDL based on design discussions.

## Core Purpose

PUDL is a **personal infrastructure knowledge base and data lake** designed to help SRE/platform engineers/software engineers navigate, learn about, and isolate issues in their cloud environments. The primary goal is **outlier detection and sprawl reduction** in cloud infrastructure and configuration.

## Architecture Overview

### Data Lake Structure
- **Schema Repository**: `~/.pudl/schema/` - Git repository containing CUE schema definitions
- **Data Storage**: Sibling directory to schema store containing:
  - Raw imported data with full provenance metadata
  - Processed data in Parquet format for performance
  - DuckDB integration for analytical queries
  - SQLite fallback when needed
- **Self-contained**: No external dependencies required for PUDL binary operation

### Two-Tier Schema System
1. **Broad Type Recognition**: Identifies what type of resource (e.g., "Kubernetes deployment", "AWS EC2 instance")
2. **Policy Compliance**: More restrictive schemas that identify resources conforming to business requirements
   - Nothing is rejected if it's a valid instance of the resource type
   - Easy identification of policy violations/outliers
   - Enables infrastructure standardization efforts

### Technology Stack
- **CUE Lang**: Primary tool for schema definition, validation, and data transformation
  - Use CUE's constraint system for data transformation
  - Keep door open for logic programming capabilities
  - Leverage new module/package system for modularity (when practical)
- **Zygomys Lisp**: Embedded rule engine for business logic
  - Schema inference rules
  - Data correlation rules
  - Data transformation rules
  - Validation rules beyond schema conformance
  - Data enrichment/correlation rules
  - User-extensible for tool integration (e.g., AWS CLI)
- **Go**: Core application with Cobra CLI framework
- **Bubble Tea**: Interactive TUI workflows for schema review and management

## Data Management

### Data Ingestion
- Support piped input: `kubectl get pods -o json | pudl import --schema k8s-pods`
- Support file/directory import: `pudl import --import-path ./aws-resources.json`
- Automatic format detection (JSON, YAML, CSV, XML, plain text)
- Full provenance tracking: timestamp, command used, origin files, etc.

### Schema Inference Workflow
1. When unrecognized data is imported, apply Zygomys rules to infer schema
2. Store data with "unconfirmed schema" status
3. Present inferred schemas to user for review (git-like workflow)
4. User can confirm inferred schema or create custom schema
5. Track schema confidence levels based on number of matching records (future feature)

### Data Versioning
- Track changes to resources over time
- Use CUE tag attributes or custom functions to determine resource identity
- Maintain historical versions of logical resources

### Data Partitioning
- Partition data for performance by source type, time, schema, or combination
- Optimize for common query patterns (outlier detection, resource lookup)

## Rule Engine & Correlation

### Zygomys Rule Engine
- All business rules written in Zygomys for extensibility
- Rule precedence and conflict resolution (TBD - will emerge during implementation)
- User-defined correlation rules between data sources
- Automatic correlation inference with user confirmation

### Cross-Source Correlation
- Enable correlation between different data sources (AWS EC2 ↔ K8s nodes)
- Compute correlations at ingestion time and/or on-demand (TBD - open to suggestions)
- Simple DSL for users to specify correlations

## User Experience

### CLI Interface
- Git-like commands for schema management: `pudl schema commit -m "msg"`, `pudl schema push/pull`
- Import commands with automatic schema detection
- Interactive workflows using Bubble Tea framework

### Schema Management
- Git-based version control for schemas
- Interactive menu-based workflows for schema review
- Metadata tracking for schema changes (possibly derived from git commits)

### Outlier Detection & Reporting
- Show all instances of resource types with outlier counts
- Requires two-tier schema system to identify what constitutes an outlier
- Focus on reducing infrastructure sprawl and identifying non-compliant resources

## Implementation Philosophy

### Development Approach
- **Small incremental steps** to avoid large rework cycles
- **Keep doors open** for future capabilities without over-engineering
- **User-friendly workflows** with interactive components
- **Minimal viable features** before expanding scope

### Testing Strategy
- Focus on simple unit tests
- Avoid test bloat and clutter
- Use mock data where necessary
- Minimize test data files in repository

### Integration Philosophy
- Primarily standalone tool serving as personal database/knowledgebase
- Allow integration with external tools via Zygomys extensions
- Users can write Zygomys scripts to leverage tools like AWS CLI, kubectl, etc.

## Future Considerations

### Advanced Features (Far Future)
- Expert system component for schema management
- Automatic detection and management of common substructures
- Advanced correlation inference
- Dashboard/reporting interfaces

### Extensibility
- Custom CUE functions defined by PUDL developers (not users)
- Possible user insertion of Zygomys functions into CUE documents (TBD)
- Rule engine extensibility for custom business logic

## Open Questions & Architecture Discussions Needed

1. **Schema Drift Handling**: Hard validation vs. soft warnings vs. automatic evolution
2. **Rule Precedence**: How to resolve conflicts when multiple rules apply
3. **Correlation Timing**: Ingestion-time vs. on-demand computation
4. **Data Partitioning Strategy**: Optimal partitioning for performance
5. **Schema Review UX**: Level of detail to show users during schema confirmation
6. **Identity Determination**: How to identify what constitutes a "change" vs. "different resource"

These architectural decisions will be made as implementation progresses and requirements become clearer.
