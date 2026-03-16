# CLI Reference

All commands support `--json` for machine-readable output and `--help` for inline documentation.

## Workspace

### `pudl init`

Initialize the PUDL workspace at `~/.pudl/`.

Creates the configuration file, data directories, and a git-tracked schema repository with bootstrap schemas. Safe to run multiple times -- skips if already initialized.

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

### `pudl setup`

Set up shell integration (aliases, completion, helper functions).

```bash
pudl setup                     # Auto-detect shell and install
pudl setup --shell bash        # Force bash setup
pudl setup --dry-run           # Show what would be added
pudl setup --uninstall         # Remove PUDL integration
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--shell` | Target shell: `bash`, `zsh`, `fish` (auto-detected if omitted) |
| `--dry-run` | Preview changes without modifying config files |
| `--uninstall` | Remove PUDL shell integration from all detected shells |

### `pudl completion`

Generate shell completion scripts.

```bash
pudl completion bash
pudl completion zsh
pudl completion fish
```

## Data Import

### `pudl import`

Import data files with automatic format detection, schema inference, and provenance tracking.

```bash
pudl import --path <file>
pudl import --path <file> --origin <source>
pudl import --path <file> --schema <schema-name>
pudl import --path <file> --format <format>
pudl import --path "*.json"                       # Wildcard batch import
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
- JSON API wrapper responses are detected and unwrapped (score >= 0.50)
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
```

### `pudl export`

Export data in various formats.

```bash
pudl export --id babod-fakak                    # Export single entry by proquint ID
pudl export --schema aws.#EC2Instance           # Export all EC2 instances
pudl export --origin k8s-pods --format yaml     # Export K8s pods as YAML
pudl export --id babod-fakak --output out.json  # Export to file
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--id` | Export entry by proquint ID |
| `--schema` | Export entries matching schema |
| `--origin` | Export entries from origin |
| `--format` | Output format: `json`, `yaml`, `csv`, `ndjson` (default: `json`) |
| `--output` / `-o` | Output file (default: stdout) |
| `--pretty` | Pretty-print output (default: true) |

At least one of `--id`, `--schema`, or `--origin` is required.

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

Validate catalog data against assigned CUE schemas.

```bash
pudl validate --entry babod-fakak      # Validate a specific entry by proquint
pudl validate --all                     # Validate all catalog entries
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--entry` | Validate a specific entry by proquint ID |
| `--all` | Validate all catalog entries |

Exactly one of `--entry` or `--all` is required.

### `pudl verify`

Verify that schema inference is a fixed point for all catalog entries. Re-runs inference on every entry and confirms each still resolves to the same schema it was originally assigned.

```bash
pudl verify
```

Any mismatch indicates drift between stored assignments and current inference rules.

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

### `pudl schema show <name>`

Print schema file contents to stdout.

### `pudl schema edit <name>`

Open a schema file in `$EDITOR`.

### `pudl schema validate`

Validate CUE schema files for correctness.

### `pudl schema reinfer`

Re-run schema inference on all existing catalog entries. Use after adding or modifying schemas to reclassify data.

```bash
pudl schema reinfer
```

### `pudl schema migrate`

Migrate schema names to canonical `<package-path>.#<Definition>` format.

### `pudl schema generate-type`

Generate a schema from a type registry (Kubernetes, AWS, GitLab).

```bash
pudl schema generate-type --kind Pod --api-version v1
```

### Schema Version Control

The schema directory is a git repository:

```bash
pudl schema status                     # Show uncommitted changes
pudl schema commit -m "Add RDS schema" # Commit changes
pudl schema log                        # Show commit history
pudl schema log --verbose              # Detailed history
```

## Schema Catalog

### `pudl catalog`

Display the schema catalog -- a central inventory of all registered schema types with their metadata.

```bash
pudl catalog              # List all registered types
pudl catalog --verbose    # Show identity fields, tracked fields, etc.
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--verbose` / `-v` | Show additional metadata fields (identity_fields, tracked_fields) |

## Definition Management

### `pudl definition list`

List available definitions.

```bash
pudl definition list
pudl definition list --verbose
```

Aliases: `pudl def list`, `pudl d list`

### `pudl definition show <name>`

Display detailed definition information including socket bindings and dependencies.

```bash
pudl definition show my_definition
pudl def show prod_instance
```

### `pudl definition validate [name]`

Validate definitions against their schemas.

```bash
pudl definition validate              # Validate all
pudl definition validate prod_instance  # Validate one
```

### `pudl definition graph`

Show the dependency graph between definitions based on socket wiring.

```bash
pudl definition graph
```

## Drift Detection

### `pudl drift check [definition]`

Compare declared definition state against live state from imported data.

```bash
pudl drift check my_instance
pudl drift check --all
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--all` | Check all definitions |

### `pudl drift report <definition>`

Display the last saved drift report without re-running detection.

```bash
pudl drift report my_instance
```

## Repository Operations

### `pudl repo init`

Initialize a `.pudl/` directory in the current repository and install Claude skills into `.claude/skills/`.

```bash
pudl repo init
pudl repo init --force    # Force reinitialize
```

### `pudl repo validate`

Validate all schemas and definitions workspace-wide.

```bash
pudl repo validate
```

Reports total definitions, validation errors, and broken socket wiring.

## Interoperability

### `pudl export-actions`

Export drift reports as mu-compatible action specs (JSON to stdout).

```bash
pudl export-actions --definition my_instance
pudl export-actions --all
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--definition` | Definition name to export actions for |
| `--all` | Export actions for all definitions with drift reports |

### `pudl data search`

Search stored artifacts by definition, method, or other criteria.

```bash
pudl data search
pudl data search --definition prod_instance
pudl data search --definition prod_instance --method list
pudl data search --limit 10
```

### `pudl data latest <definition> <method>`

Show the most recent artifact for a definition/method pair.

```bash
pudl data latest prod_instance list
pudl data latest prod_instance list --raw
```

## Vault

### `pudl vault get <path>`

Retrieve a secret from the vault.

```bash
pudl vault get aws/access_key
```

### `pudl vault set <path> <value>`

Store a secret in the vault (file backend only).

```bash
pudl vault set aws/access_key "AKIA..."
```

### `pudl vault list`

List stored secret paths.

```bash
pudl vault list
```

### `pudl vault rotate-key`

Re-encrypt the file vault with a new passphrase.

```bash
pudl vault rotate-key
```

## Migration

### `pudl migrate identity`

Backfill `resource_id`, `content_hash`, and `version` columns for catalog entries created before identity tracking was added.

```bash
pudl migrate identity
```

## CUE Module Management

### `pudl module`

Manage CUE module dependencies for the schema repository.

```bash
pudl module tidy              # Fetch and update module dependencies
pudl module list              # List current module dependencies
pudl module info              # Show module information
pudl module add <module@ver>  # Add a third-party module dependency
```
