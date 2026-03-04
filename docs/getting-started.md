# Getting Started

## Prerequisites

- Go 1.24+ (for building from source)
- Git (for schema version control)

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

## Next Steps

- Read [concepts.md](concepts.md) to understand the identity system and schema inference
- Read [schema-authoring.md](schema-authoring.md) to write custom schemas
- Read [collections.md](collections.md) for advanced collection workflows
- Run `pudl --help` or `pudl <command> --help` for inline CLI help
