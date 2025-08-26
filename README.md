# Personal Unified Data Lake

PUDL is a golang CLI tool which aims to help those who work with cloud resources amplify their ability to leverage data as part of their regular workflows. It manages the import, querying, processing and updating of a local 'data lake' comprising data on remote resources such as AWS or GCP resources, Kubernetes resources, logs, metrics, et cetera. It operates according to a few basic principles

* Schema are managed using cuelang. All schema are kept under version control in a git repository, stored by default at ~/.pudl/schema/

* Users can create schema by hand from scratch, or use cuelang's import abilities to import schema from any data source the cue cli is capable of importing. They can also have the pudl cli scaffold a new schema based on an input datum. When the user asks pudl to create a new schema from sample data, pudl executes a rule engine to determine how to model the schema

* All business rules in the pudl system are written in zygomys, a lisp which can be embedded in golang. This includes the rule engine used to infer and generate schema from sample data and other business logic where it is advantageous to provide extensibility.

* Users can import data by piping it into the pudl cli, or by using `pudl import --import-path <path>` where path is a file, directory or glob. On importing, if a schema is not specified with the `--schema` flag, and if the user does not deactivate schema inferrence via `--disable-schema-inferrence`, pudl will first determine the format of the data (json, yaml, csv, xml, plain string, etc) and deserailize it if necessary, then attempt to infer the schema of the data being imported by applying a series of production rules and checking the data against a registry of all schema in the schema repository. For example, it will determine whether the data, if it was deserialized, is a collection of items or an item, and use this fact to narrow down the possible schema. To reiterate, these production rules are written in zygmoys so as to provide a simple, uniform, powerful, maintainble means of extending functionality.

## Current Status

**Phase 2 Complete** - PUDL now has a fully functional data storage and retrieval system!

### ✅ Currently Implemented

**Core Functionality:**
- **Data Import**: `pudl import --path <file>` with automatic format detection and schema assignment
- **Data Discovery**: `pudl list` with filtering by schema, origin, format and sorting options
- **Data Inspection**: `pudl show <id>` with detailed metadata and raw data display
- **Schema System**: Auto-created CUE schemas for AWS, Kubernetes, and unknown data types

**Technical Features:**
- Automatic format detection (JSON, YAML, CSV)
- Intelligent origin detection from filenames
- Rule-based schema assignment with confidence scoring
- Date-partitioned storage with immutable raw data
- Comprehensive metadata tracking and data catalog
- Human-readable output with filtering and sorting

**Example Usage:**
```bash
# Initialize workspace
pudl init

# Import cloud infrastructure data
pudl import --path aws-ec2-instances.json
pudl import --path k8s-pods.yaml

# Discover and filter data
pudl list --schema aws.#EC2Instance --verbose
pudl list --format yaml --sort-by size

# Inspect specific entries
pudl show 20250825_222545_aws-ec2-describe-instances --raw
```

See `plan.md` for the detailed implementation roadmap and `implementation_log_2025_08_25.md` for recent development progress.
