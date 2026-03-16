# Getting Started

## Prerequisites

- Go 1.24+ (for building from source)
- Git (for schema version control)

## 1. Install and Initialize

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

## 2. Import Some Data

```bash
pudl import --path aws-ec2-instances.json
```

PUDL will:
1. Hash the file contents (SHA256) for deduplication
2. Detect the format (JSON, NDJSON, YAML, or CSV)
3. Infer the best matching CUE schema
4. Extract resource identity fields
5. Store the raw file and metadata
6. Add a catalog entry

Output looks like:

```
Imported: aws-ec2-instances.json
   ID:       mivof-duhij
   Schema:   aws/ec2.#Instance (confidence: 0.85)
   Format:   json
   Records:  1
```

You can override detection when needed:

```bash
# Specify origin manually
pudl import --path data.json --origin my-custom-source

# Specify schema explicitly (skips inference)
pudl import --path data.json --schema aws/ec2.#Instance
```

NDJSON files and JSON API wrappers are automatically detected and split into collections:

```bash
pudl import --path cloud-inventory.json
# Output: Detected format: ndjson
#         Created collection with 832 items
```

## 3. List and Show Entries

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

# Collections only, or items from a specific collection
pudl list --collections-only
pudl list --collection-id cloud-inventory
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

### View the schema catalog

```bash
# List all registered schema types
pudl catalog

# With additional metadata
pudl catalog --verbose
```

## 4. Write a Schema

Generate a schema from existing data:

```bash
# Generate from an imported entry
pudl schema new --from mivof-duhij --path mypackage/#MyResource
```

Or write one by hand. A PUDL schema is a CUE definition with a `_pudl` metadata block:

```cue
package mypackage

#MyResource: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "mypackage.myresource"
        identity_fields: ["id"]
        tracked_fields:  ["status", "name"]
    }
    id:     string
    name:   string
    status: string
    ...
}
```

Add it to the schema repository:

```bash
pudl schema add mypackage.my-resource my-schema.cue
pudl schema status    # See uncommitted changes
pudl schema commit -m "Add custom resource schema"
```

After adding schemas, re-classify existing entries:

```bash
pudl schema reinfer
```

See [schema-authoring.md](schema-authoring.md) for the full guide on writing schemas.

## 5. Validate Data Against Schemas

Check whether imported data conforms to its assigned schema:

```bash
# Validate a specific entry by proquint ID
pudl validate --entry mivof-duhij

# Validate all catalog entries
pudl validate --all

# Validate all with detailed output
pudl validate --all --verbose
```

Validation uses native CUE unification. If the assigned schema rejects the data, PUDL falls through to the base schema, then to the catchall. Data is never rejected outright.

## 6. Create a Definition

Definitions are named CUE values that conform to schemas with concrete configuration and socket wiring to other definitions.

```bash
# List definitions
pudl definition list

# Show a specific definition
pudl definition show prod_instance

# Validate all definitions against their schemas
pudl definition validate

# View the dependency graph
pudl definition graph
```

See [definition-authoring.md](definition-authoring.md) for the full guide.

## 7. Check for Drift

Compare a definition's declared state against the actual imported data:

```bash
# Check a single definition
pudl drift check prod_instance

# Check all definitions
pudl drift check --all

# View a saved drift report
pudl drift report prod_instance
```

Drift reports can be exported as mu-compatible action specs:

```bash
pudl export-actions --definition prod_instance
pudl export-actions --all
```

## 8. Run Verification

Verify that schema inference is a fixed point -- re-running inference on all catalog entries produces the same schema assignments:

```bash
pudl verify
```

Any mismatches indicate drift between the stored schema and the current inference rules. This is a correctness invariant for ensuring inference determinism.

## Other Useful Commands

### Delete entries

```bash
pudl delete mivof-duhij
pudl delete mivof-duhij --force
pudl delete govim-nupab --cascade --force   # Delete collection and all items
```

### Configuration

```bash
pudl config                          # View current configuration
pudl config set data_path ~/my-data  # Change a setting
pudl config reset                    # Reset to defaults
```

### Health check

```bash
pudl doctor
```

Checks workspace structure, database integrity, schema repository setup, git initialization, directory structure, and orphaned files.

### Large files

All imports use streaming by default. For very large files:

```bash
pudl import --path huge-file.json --streaming-memory 500
pudl import --path massive-data.json --streaming-chunk-size 0.064
```

## Next Steps

- Read [concepts.md](concepts.md) to understand the identity system and schema inference
- Read [schema-authoring.md](schema-authoring.md) to write custom schemas
- Read [collections.md](collections.md) for advanced collection handling
- Run `pudl --help` or `pudl <command> --help` for inline CLI help
