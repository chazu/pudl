# Schema Authoring Guide

PUDL schemas are [CUE](https://cuelang.org/) files that define both the **shape** of your data and the **metadata** that drives schema inference, identity tracking, and validation.

## Anatomy of a Schema

```cue
package ec2

#Instance: {
    // PUDL metadata -- drives inference, identity, and validation
    _pudl: {
        schema_type:     "base"
        resource_type:   "aws.ec2.instance"
        identity_fields: ["InstanceId"]
        tracked_fields:  ["State", "InstanceType", "Tags"]
    }

    // Data shape -- standard CUE constraints
    InstanceId:   string
    InstanceType: string
    State: {
        Name: "pending" | "running" | "stopping" | "stopped" | "terminated"
        ...
    }
    Tags?: [...{Key: string, Value: string}]

    // Accept additional fields not listed above
    ...
}
```

Every PUDL schema has two parts:
1. A `_pudl` metadata block that tells the inference engine how to use this schema
2. CUE field constraints that define what valid data looks like

## The `_pudl` Metadata Block

The valid metadata fields are described below. There are no priority numbers or fallback lists -- validation order is determined by native CUE unification and the schema inheritance graph.

### `schema_type`

Declares the role of this schema in the validation hierarchy.

| Value | Purpose |
|-------|---------|
| `"base"` | Type recognition -- matches any valid instance of this resource type |
| `"policy"` | Compliance checking -- adds constraints beyond type recognition |
| `"custom"` | User-defined schemas outside the type/policy hierarchy |
| `"catchall"` | Universal fallback that accepts anything |
| `"collection"` | Schema for collection entries (not individual items) |

### `resource_type`

A dotted identifier describing what kind of resource this schema represents. Used for origin-matching during inference.

Examples: `"aws.ec2.instance"`, `"k8s.pod"`, `"gitlab.pipeline"`, `"generic.collection"`

When PUDL imports a file named `aws-ec2-instances.json`, the origin `aws-ec2-instances` is matched against `resource_type` values. Two or more keyword matches (e.g., "aws" and "ec2") boost the schema's candidate score by +0.15.

### `identity_fields`

A list of field names that **uniquely identify a logical resource**. These serve two purposes:

1. **Inference**: If all identity fields are present in the data, the schema gets a strong +0.5 score boost
2. **Resource identity**: The values of these fields (combined with the schema name) produce the `resource_id` hash, which tracks the same logical resource across re-imports

```cue
identity_fields: ["InstanceId"]                                  // EC2 instance
identity_fields: ["metadata.name", "metadata.namespace"]         // K8s resource
identity_fields: ["BucketName"]                                  // S3 bucket
```

If `identity_fields` is empty (as in the catchall), the content hash is used as the identity component.

### `tracked_fields`

A list of fields to **monitor for changes** between versions of the same resource. These provide a weaker inference signal (+0.1 x ratio of present fields) and are used for diff/change-tracking features.

```cue
tracked_fields: ["State", "InstanceType", "Tags"]
```

### `base_schema` (optional)

Reference to a parent schema. This builds an inheritance graph used for specificity ordering -- child schemas (more specific) are tried before parent schemas (more generic).

```cue
// A compliant EC2 instance inherits from the base EC2 instance schema
#CompliantInstance: #Instance & {
    _pudl: {
        schema_type:  "policy"
        base_schema:  "aws/ec2.#Instance"
    }

    // Additional compliance requirements
    Tags: [...{Key: string, Value: string}] & list.MinItems(1)
}
```

## How Validation Works

When data is imported, PUDL determines which schema to assign using a two-phase process:

### Phase 1: Candidate Selection (Heuristic Scoring)

Candidate schemas are scored based on:
- **Identity field presence**: +0.5 if all `identity_fields` exist in the data
- **Tracked field presence**: +0.1 x (ratio of `tracked_fields` found)
- **Origin keyword matching**: +0.15 if two or more `resource_type` keywords match the origin
- **Schema specificity**: Candidates are sorted most-specific-first using the `base_schema` inheritance graph

### Phase 2: CUE Unification

Starting with the most specific candidate, PUDL attempts CUE unification (`tryUnify`) against the data. The first schema that successfully unifies is assigned.

If no candidate passes unification, PUDL walks the `base_schema` chain -- trying progressively less specific schemas. If the entire chain fails, the catchall schema (`pudl/core.#Item`) is assigned.

This means:
- **No priority numbers** -- ordering comes from the inheritance graph and heuristic scores
- **No explicit fallback lists** -- the `base_schema` chain provides natural fallback
- **Data is never rejected** -- the catchall always accepts

## Resource Identity and Schema Families

A schema's role is *classification*; it is distinct from a resource's *identity*.
The same logical resource (a repo, an instance) may be classified under several
schemas in an inheritance family and may be reclassified over time, but it should
keep one stable `resource_id` so data from different sources can be linked and
deduplicated.

To make that true, `resource_id` is namespaced by the **root of the schema's
inheritance family**, not the assigned (leaf) schema:

```
resource_id = SHA256( normalize(family_root) + "\x00" + identity_component )
```

where `identity_component` is the canonical JSON of the identity field values
(or, for catchall schemas with no identity fields, the content hash). Because the
family root is invariant under reinference and policy/specialization refinement
(those move the assigned leaf, never the root), a resource's identity stays
stable even as its classification changes.

### The family identity invariant

For dedup to be correct, every schema a single resource can be classified under
must extract the **same identity values from the same fields**. Therefore:

> `identity_fields` must be identical across an inheritance family. Declare them
> at the family root and inherit them unchanged. Descendants may *tighten
> constraints* on those fields (e.g. narrow `id: string` to a pattern) but must
> not change the field set or the values extracted.

When you build a family with CUE unification (`#Child: #Base & {...}`), CUE
enforces this automatically — divergent `identity_fields` lists fail to unify.
`pudl doctor` includes an **Identity Fields** check that backstops `base_schema`
references which bypass CUE inheritance, warning when a schema's
`identity_fields` differ from its base's.

Choose identity fields that are globally unique where possible (URLs, ARNs,
UUIDs, fully-qualified paths). The family-root namespace prevents cross-family
collisions even when identity fields are weak, but good identity fields make
identity robust regardless.

### After upgrading to family-root namespacing

Family-root namespacing changes every `resource_id`. Run the recompute migration
once after upgrading to bring existing entries onto the new scheme (it also
re-sequences versions per resource):

```bash
pudl migrate identity --recompute --dry-run   # preview
pudl migrate identity --recompute             # apply (idempotent)
```

## Creating a Schema from Data

The fastest way to start is generating a schema from an existing import:

```bash
# Import some data first
pudl import --path my-data.json

# Generate a schema from it (under the recommended `user` namespace)
pudl schema new --from mivof-duhij --path user/#MyResource
```

This creates a CUE file with fields inferred from the data. You will want to edit it to:
1. Add the `_pudl` metadata block
2. Tighten constraints (replace `_` with specific types)
3. Mark optional fields with `?`
4. Add `...` if extra fields should be allowed

### Enum Inference

If a field has a small set of known values, you can infer it as an enum:

```bash
pudl schema new --from mivof-duhij --path user/#MyResource --infer status=enum
```

This generates a CUE disjunction like `status: "active" | "pending" | "inactive"` based on the values seen in the data.

### Collection Schemas

When generating a schema from a collection entry, use `--collection` to generate a schema for the item type rather than the collection wrapper:

```bash
pudl schema new --from govim-nupab --collection --path user/#MyItem
```

## Schema File Location

Schemas live in `~/.pudl/schema/` organized by package (directory). The schema name format is `<package-path>.#<Definition>`:

```
~/.pudl/schema/
+-- cue.mod/module.cue
+-- pudl/                          # reserved for built-in schemas (see below)
|   +-- core/core.cue             # pudl/core.#Item, pudl/core.#Collection
|   +-- aws/
|   |   +-- ec2.cue               # pudl/aws/ec2.#Instance
|   |   +-- s3.cue                # pudl/aws/s3.#Bucket
|   +-- k8s/
|       +-- resources.cue         # pudl/k8s.#Pod, pudl/k8s.#Service, etc.
+-- user/                         # recommended namespace for your schemas
    +-- git.cue                   # user/git.#Repository
    +-- k8s.cue                   # user/k8s.#CustomResource
```

### Namespace Convention

PUDL does not enforce any particular package path for your own schemas -- the package path is simply the directory you place a `.cue` file under. To keep things organized and avoid collisions, follow this convention:

- **Use `user` as your top-level package by default.** Put bespoke schemas under `~/.pudl/schema/user/`, producing names like `user/git.#Repository` or `user/k8s.#CustomResource`. This namespaces all of your schemas together and keeps them clearly separated from PUDL's built-ins.
- **Any name of your choice is valid.** If `user` does not fit, pick something meaningful -- an org or team name (`acme/`, `platform/`), a domain (`finance/`), or a product (`myapi/`). PUDL applies no restrictions to the package path beyond it being a valid CUE package.
- **Use multiple top-level names freely.** You are not limited to one. Mix `user/`, `acme/`, and `experimental/` side by side; each is an independent package path.
- **Avoid the `pudl/` namespace.** It is reserved for built-in schemas, and the legacy short form `core.#Item` auto-normalizes to `pudl/core.#Item`. Placing your schemas under `pudl/` risks shadowing built-ins (first-found-wins), so keep your packages outside it.

Whatever names you choose, the CUE `package` declaration inside each file must match its package directory.

When adding a schema manually:

```bash
pudl schema add user.my-resource my-schema.cue
```

The CUE `package` declaration in the file must match the target package directory.

## Version Control

The schema directory is a git repository. Use PUDL's built-in commands:

```bash
pudl schema status                     # Show uncommitted changes
pudl schema commit -m "Add EC2 schema" # Commit
pudl schema log                        # View history
```

## After Adding or Changing Schemas

Run `pudl schema reinfer` to re-classify existing catalog entries against the updated schemas:

```bash
pudl schema reinfer
```

This re-runs inference on all entries and updates their schema assignments. Entries that previously fell through to the catchall may now match your new, more specific schema.

You can also verify that inference is stable after changes:

```bash
pudl verify
```

This confirms that re-running inference on every entry produces the same schema assignment it already has.

## Example: Full Custom Schema

Here is a complete example for a custom API resource:

```cue
package myapi

import "list"

// User represents a user account from the internal API
#User: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "myapi.user"
        identity_fields: ["id"]
        tracked_fields:  ["email", "role", "last_login"]
    }

    id:         string & =~"^usr_[a-zA-Z0-9]+$"
    email:      string
    role:       "admin" | "editor" | "viewer"
    last_login: string
    created_at: string

    // Allow extra fields
    ...
}

// AdminUser is a policy schema -- a User with admin-specific requirements
#AdminUser: #User & {
    _pudl: {
        schema_type:  "policy"
        base_schema:  "myapi.#User"
    }

    role: "admin"
    mfa_enabled: true
}
```

After adding this file:

```bash
pudl schema add myapi.users users.cue
pudl schema commit -m "Add user schemas"
pudl schema reinfer  # Re-classify existing imports
```

## Tips

- **Start permissive, tighten later**: Use `...` to accept extra fields, then constrain as you learn your data shape
- **Identity fields matter most**: Good identity fields enable version tracking and strong inference. Choose fields that uniquely identify a resource instance.
- **Use `pudl schema reinfer`** after any schema change to re-classify existing data
- **Check inference results**: After importing, look at the confidence score. Low confidence (< 0.5) means the schema is a weak match -- consider adding more identity fields or refining the schema structure.
- **Use `base_schema` for inheritance**: Policy schemas should reference their base type via `base_schema` so the validation cascade walks the inheritance chain correctly.
