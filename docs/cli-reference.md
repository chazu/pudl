# CLI Reference

All commands support `--json` for machine-readable output and `--help` for inline documentation.

## Workspace

### `pudl init`

Initialize the PUDL workspace at `~/.pudl/`.

Creates the configuration file, data directories, and a git-tracked schema repository with bootstrap schemas. Safe to run multiple times — skips if already initialized.

```bash
pudl init
pudl init --force  # Reinitialize (preserves existing data)
```

### `pudl config`

View or modify configuration.

```bash
pudl config                          # Show current configuration
pudl config set data_path ~/my-data  # Change a setting
pudl config reset                    # Reset to defaults
```

### `pudl version`

Show version, commit, and build date.

### `pudl doctor`

Run health checks on the PUDL workspace.

## Data Import

### `pudl import`

Import data files with automatic format detection, schema inference, and provenance tracking.

```bash
pudl import --path <file>
pudl import --path <file> --origin <source>
pudl import --path <file> --schema <schema-name>
pudl import --path <file> --format <format>  # For stdin
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--path` | File to import (required). Supports wildcards. Use `-` for stdin. |
| `--origin` | Override automatic origin detection |
| `--schema` | Specify schema explicitly (skip inference) |
| `--format` | Specify format when reading from stdin (`json`, `yaml`, `csv`, `ndjson`) |
| `--streaming-memory` | Memory limit in MB for streaming (default: 100) |
| `--streaming-chunk-size` | Chunk size in MB for streaming (default: 0.016) |

**Behavior:**

- Duplicate files (same content hash) are skipped automatically
- NDJSON files are split into collections with individual items
- JSON API wrapper responses are detected and unwrapped (score ≥ 0.50)
- Format is detected from extension and content analysis
- Origin is inferred from filename patterns

**Debug mode:** Set `PUDL_DEBUG=1` for detailed error output.

## Data Discovery

### `pudl list`

Query and filter catalog entries.

```bash
pudl list
pudl list --schema aws/ec2.#Instance
pudl list --origin k8s --format yaml
pudl list --sort-by size --reverse --limit 10
pudl list --collections-only
pudl list --collection-id my-inventory --schema aws/ec2.#Instance
pudl list --fancy  # Interactive TUI
```

**Filter flags:**

| Flag | Description |
|------|-------------|
| `--schema` | Filter by schema name (partial match, case-insensitive) |
| `--origin` | Filter by origin (partial match, case-insensitive) |
| `--format` | Filter by format (`json`, `yaml`, `csv`, `ndjson`) |
| `--collections-only` | Show only collection entries |
| `--items-only` | Show only individual item entries |
| `--collection-id` | Show items from a specific collection |
| `--item-id` | Show a specific item by its item ID |

**Display flags:**

| Flag | Description |
|------|-------------|
| `--verbose` | Show file paths, identity info, and summary statistics |
| `--limit` | Maximum number of entries to display |
| `--sort-by` | Sort field: `timestamp`, `size`, `records`, `schema`, `origin`, `format` |
| `--reverse` | Reverse sort order |
| `--page` | Page number for pagination |
| `--per-page` | Entries per page |
| `--fancy` | Launch interactive TUI (bubbletea) |

### `pudl show <id>`

Inspect a specific catalog entry. Accepts proquint IDs (`mivof-duhij`) or full hex hashes.

```bash
pudl show mivof-duhij
pudl show mivof-duhij --raw         # Show raw data content
pudl show mivof-duhij --metadata    # Show import metadata
pudl show mivof-duhij --validation  # Show validation results
```

### `pudl export`

Export data in various formats.

## Data Management

### `pudl delete <id>`

Remove a catalog entry and its associated files (raw data + metadata).

```bash
pudl delete mivof-duhij              # With confirmation prompt
pudl delete mivof-duhij --force      # Skip confirmation
pudl delete govim-nupab --cascade    # Delete collection + all items
pudl delete mivof-duhij --json       # JSON output for scripting
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--force` | Skip confirmation prompt |
| `--cascade` | Delete collection and all its child items |
| `--json` | Output results as JSON |

Collections with items cannot be deleted without `--cascade`. Individual items can always be deleted.

### `pudl validate`

Validate data against CUE schemas.

## Schema Management

### `pudl schema list`

List available schemas.

```bash
pudl schema list
pudl schema list --package aws
pudl schema list --package k8s --verbose
```

### `pudl schema add <name> <file>`

Add a CUE schema file to the repository.

```bash
pudl schema add aws.rds-instance my-rds-schema.cue
pudl schema add custom.api-response api-schema.cue
```

### `pudl schema new`

Generate a CUE schema from imported data.

```bash
pudl schema new --from <id> --path <package>#<Definition>
pudl schema new --from mivof-duhij --path mypackage/#MyResource
pudl schema new --from mivof-duhij --path mypackage/#MyResource --infer status=enum
pudl schema new --from govim-nupab --collection --path mypackage/#MyItem
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--from` | Catalog entry ID to generate schema from |
| `--path` | Target schema path (`package/#Definition`) |
| `--collection` | Generate schema for collection items (not the wrapper) |
| `--infer <field>=enum` | Infer field as enum from observed values |

### `pudl schema generate-type`

Generate a schema from a type registry (Kubernetes, AWS, GitLab).

```bash
pudl schema generate-type --kind Pod --api-version v1
```

### `pudl schema show <name>`

Print schema file contents to stdout.

### `pudl schema edit <name>`

Open a schema file in `$EDITOR`.

### `pudl schema reinfer`

Re-run schema inference on all existing catalog entries. Use after adding or modifying schemas to reclassify data.

```bash
pudl schema reinfer
```

### `pudl schema migrate`

Migrate schema names to canonical `<package-path>.#<Definition>` format.

### Schema Version Control

The schema directory is a git repository:

```bash
pudl schema status                     # Show uncommitted changes
pudl schema commit -m "Add RDS schema" # Commit changes
pudl schema log                        # Show commit history
pudl schema log --verbose              # Detailed history
```

## Migration

### `pudl migrate identity`

Backfill `resource_id`, `content_hash`, and `version` columns for catalog entries created before identity tracking was added.

```bash
pudl migrate identity
```

## Legacy

### `pudl process <file.cue>`

Process a CUE file with custom functions. Legacy feature from early development.

### `pudl module`

Thin wrapper around `cue mod` commands.
