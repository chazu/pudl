# Getting Started

## Prerequisites

- Go 1.24+ (for building from source)
- Git (for schema version control)
- CUE (cuelang.org) for schema definitions

## Install and Initialize

```bash
# Build PUDL
go build -o pudl .

# Initialize your workspace (creates ~/.pudl/)
./pudl init
```

This creates the workspace at `~/.pudl/` with:
- A configuration file (`config.yaml`)
- A git-tracked schema repository with bootstrap schemas
- Data storage directories

If you forget to run `init`, PUDL auto-initializes on first use.

## Import Your First File

```bash
pudl import --path aws-ec2-instances.json
```

PUDL will:
1. Hash the file contents (SHA256) for deduplication
2. Detect the format (JSON in this case)
3. Infer the best matching CUE schema
4. Extract resource identity fields
5. Store the raw file and metadata
6. Add a catalog entry

Output looks like:

```
✅ Imported: aws-ec2-instances.json
   ID:       mivof-duhij
   Schema:   aws/ec2.#Instance (confidence: 0.85)
   Format:   json
   Records:  1
```

### Supported Formats

- **JSON** (`.json`) — single objects or arrays
- **NDJSON** (`.json` with newline-delimited objects) — automatically split into collections
- **YAML** (`.yaml`, `.yml`)
- **CSV** (`.csv`)

### Override Detection

```bash
# Specify origin manually
pudl import --path data.json --origin my-custom-source

# Specify schema explicitly (skips inference)
pudl import --path data.json --schema aws/ec2.#Instance
```

## Query Your Catalog

### List entries

```bash
# All entries
pudl list

# Filter by schema, origin, or format
pudl list --schema aws/ec2.#Instance
pudl list --origin k8s --format yaml

# Sort and limit
pudl list --sort-by size --reverse --limit 10

# Verbose mode with file paths and statistics
pudl list --verbose

# Interactive TUI
pudl list --fancy
```

### Inspect an entry

```bash
# Summary view
pudl show mivof-duhij

# Show raw data
pudl show mivof-duhij --raw

# Show import metadata
pudl show mivof-duhij --metadata
```

## Import a Collection

NDJSON files are automatically detected and split into individual items:

```bash
pudl import --path cloud-inventory.json
# Output: Detected format: ndjson
#         Created collection with 832 items
```

JSON API responses with wrapper patterns are also automatically unwrapped:

```bash
# If the file contains {"items": [...], "count": 5, "next_token": "abc"}
pudl import --path api-response.json
# Output: Detected collection wrapper (score: 0.75)
#         Created collection with 5 items
```

Query collections:

```bash
# List collections only
pudl list --collections-only

# List items from a specific collection
pudl list --collection-id cloud-inventory

# Find a specific schema across all collections
pudl list --schema aws/ec2.#Instance --items-only
```

## Create a Custom Schema

Generate a schema from existing data:

```bash
# Generate from an imported entry
pudl schema new --from mivof-duhij --path mypackage/#MyResource
```

Or add a handwritten schema:

```bash
pudl schema add mypackage.my-resource my-schema.cue
pudl schema status    # See uncommitted changes
pudl schema commit -m "Add custom resource schema"
```

After adding schemas, re-classify existing entries:

```bash
pudl schema reinfer
```

See [schema-authoring.md](schema-authoring.md) for how to write effective schemas.

## Delete Entries

```bash
# Delete a single entry
pudl delete mivof-duhij

# Delete without confirmation prompt
pudl delete mivof-duhij --force

# Delete a collection and all its items
pudl delete govim-nupab --cascade --force
```

## Configure

```bash
# View current configuration
pudl config

# Change a setting
pudl config set data_path ~/my-pudl-data

# Reset to defaults
pudl config reset
```

## Health Check

```bash
pudl doctor
```

## Large Files

All imports use streaming (Content-Defined Chunking) by default. For very large files, tune memory usage:

```bash
pudl import --path huge-file.json --streaming-memory 500
pudl import --path massive-data.json --streaming-chunk-size 0.064
```

## Define a Model

Models compose CUE schemas with operational behavior — methods, sockets, authentication, and metadata.

```bash
# List available models
pudl model list

# Search for models by keyword
pudl model search ec2

# Generate a model scaffold
pudl model scaffold myservice --category custom --methods list,create --auth bearer

# View model details
pudl model show pudl/model/examples.#EC2InstanceModel
```

See [model-authoring.md](model-authoring.md) for the full guide.

## Create a Definition

Definitions are named instances of models with concrete configuration:

```bash
# List definitions
pudl definition list

# Validate all definitions
pudl definition validate

# View the dependency graph
pudl definition graph
```

See [definition-authoring.md](definition-authoring.md) for the full guide.

## Run a Method

Methods execute operations against definitions with lifecycle dispatch:

```bash
# Execute a method
pudl method run prod_instance list

# Dry run (qualifications only)
pudl method run prod_instance create --dry-run

# Pass extra arguments
pudl method run prod_instance create --tag env=staging

# List available methods for a definition
pudl method list prod_instance
```

See [method-authoring.md](method-authoring.md) for the full guide.

## Run a Workflow

Workflows orchestrate multiple method executions as a DAG:

```bash
# Run a workflow
pudl workflow run deploy-stack

# Validate before running
pudl workflow validate deploy-stack

# View workflow details
pudl workflow show deploy-stack

# Check run history
pudl workflow history deploy-stack
```

See [workflows.md](workflows.md) for more.

## Check for Drift

Compare declared infrastructure state against live state:

```bash
# Check a single definition
pudl drift check prod_instance

# Check all definitions
pudl drift check --all

# Re-execute method before comparing
pudl drift check prod_instance --refresh

# View last saved report
pudl drift report prod_instance
```

See [drift.md](drift.md) for more.

## Manage Secrets

The vault stores credentials used by definitions and methods:

```bash
# Store a secret
pudl vault set aws/access_key "AKIA..."

# Retrieve a secret
pudl vault get aws/access_key

# List stored paths
pudl vault list
```

Vault references in definitions (`vault://aws/access_key`) are resolved at execution time. See [vault.md](vault.md) for more.

## Next Steps

- Read [concepts.md](concepts.md) to understand the identity system and schema inference
- Read [schema-authoring.md](schema-authoring.md) to write custom schemas
- Read [collections.md](collections.md) for advanced collection workflows
- Read [model-authoring.md](model-authoring.md) to write models
- Read [definition-authoring.md](definition-authoring.md) to write definitions
- Read [method-authoring.md](method-authoring.md) to write methods
- Read [workflows.md](workflows.md) for workflow orchestration
- Read [drift.md](drift.md) for drift detection
- Read [vault.md](vault.md) for credential management
- Run `pudl --help` or `pudl <command> --help` for inline CLI help
