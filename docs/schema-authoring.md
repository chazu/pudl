# Schema Authoring Guide

PUDL schemas are [CUE](https://cuelang.org/) files that define both the **shape** of your data and the **metadata** that drives schema inference, identity tracking, and cascade validation.

## Anatomy of a Schema

```cue
package ec2

#Instance: {
    // PUDL metadata — drives inference, identity, and cascading
    _pudl: {
        schema_type:      "base"
        resource_type:    "aws.ec2.instance"
        cascade_priority: 100
        identity_fields:  ["InstanceId"]
        tracked_fields:   ["State", "InstanceType", "Tags"]
    }

    // Data shape — standard CUE constraints
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

### `schema_type`

Declares the role of this schema in the cascade hierarchy.

| Value | Purpose |
|-------|---------|
| `"base"` | Type recognition — matches any valid instance of this resource type |
| `"policy"` | Compliance checking — adds constraints beyond type recognition |
| `"custom"` | User-defined schemas outside the type/policy hierarchy |
| `"catchall"` | Universal fallback that accepts anything |
| `"collection"` | Schema for collection entries (not individual items) |

### `resource_type`

A dotted identifier describing what kind of resource this schema represents. Used for origin-matching during inference.

Examples: `"aws.ec2.instance"`, `"k8s.pod"`, `"gitlab.pipeline"`, `"generic.collection"`

When PUDL imports a file named `aws-ec2-instances.json`, the origin `aws-ec2-instances` is matched against `resource_type` values. Two or more keyword matches (e.g., "aws" and "ec2") boost the schema's candidate score by +0.15.

### `cascade_priority`

An integer controlling the order in which schemas are tried during inference and cascade validation. **Higher values = more specific = tried first.**

Recommended ranges:

| Range | Use |
|-------|-----|
| 0 | Catchall (accepts everything) |
| 50–75 | Generic / collection schemas |
| 80–100 | Base type schemas (e.g., "any EC2 instance") |
| 101–150 | Policy schemas (e.g., "compliant EC2 instance with tags") |

### `identity_fields`

A list of field names that **uniquely identify a logical resource**. These serve two purposes:

1. **Inference**: If all identity fields are present in the data, the schema gets a strong +0.5 score boost
2. **Resource identity**: The values of these fields (combined with the schema name) produce the `resource_id` hash, which tracks the same logical resource across re-imports

```cue
identity_fields: ["InstanceId"]           // EC2 instance
identity_fields: ["metadata.name", "metadata.namespace"]  // K8s resource
identity_fields: ["BucketName"]           // S3 bucket
```

If `identity_fields` is empty (as in the catchall), the content hash is used as the identity component.

### `tracked_fields`

A list of fields to **monitor for changes** between versions of the same resource. These provide a weaker inference signal (+0.1 × ratio of present fields) and will be used for future diff/change-tracking features.

```cue
tracked_fields: ["State", "InstanceType", "Tags"]
```

### `base_schema` (optional)

Reference to a parent schema. This builds an inheritance graph used for specificity ordering — child schemas (more specific) are tried before parent schemas (more generic).

```cue
// A compliant EC2 instance inherits from the base EC2 instance schema
#CompliantInstance: #Instance & {
    _pudl: {
        schema_type:      "policy"
        base_schema:      "aws/ec2.#Instance"
        cascade_priority: 120
    }

    // Additional compliance requirements
    Tags: [...{Key: string, Value: string}] & list.MinItems(1)
}
```

### `cascade_fallback` (optional)

An explicit ordered list of schemas to try if this schema fails validation. If not specified, the cascade follows the inheritance graph.

```cue
cascade_fallback: ["aws/ec2.#Instance", "pudl/core.#Item"]
```

### `compliance_level` (optional)

Hint for how strictly this schema should be applied. Currently informational.

| Value | Meaning |
|-------|---------|
| `"strict"` | All fields must match exactly |
| `"permissive"` | Extra fields are allowed (the `...` in CUE) |

## Creating a Schema from Data

The fastest way to start is generating a schema from an existing import:

```bash
# Import some data first
pudl import --path my-data.json

# Generate a schema from it
pudl schema new --from mivof-duhij --path mypackage/#MyResource
```

This creates a CUE file with fields inferred from the data. You'll want to edit it to:
1. Add the `_pudl` metadata block
2. Tighten constraints (replace `_` with specific types)
3. Mark optional fields with `?`
4. Add `...` if extra fields should be allowed

### Enum Inference

If a field has a small set of known values, you can infer it as an enum:

```bash
pudl schema new --from mivof-duhij --path mypackage/#MyResource --infer status=enum
```

This generates a CUE disjunction like `status: "active" | "pending" | "inactive"` based on the values seen in the data.

### Collection Schemas

When generating a schema from a collection entry, use `--collection` to generate a schema for the item type rather than the collection wrapper:

```bash
pudl schema new --from govim-nupab --collection --path mypackage/#MyItem
```

## Schema File Location

Schemas live in `~/.pudl/schema/` organized by package (directory). The schema name format is `<package-path>.#<Definition>`:

```
~/.pudl/schema/
├── cue.mod/module.cue
└── pudl/
    ├── core/core.cue              # pudl/core.#Item, pudl/core.#Collection
    ├── aws/
    │   ├── ec2.cue                # aws/ec2.#Instance
    │   └── s3.cue                 # aws/s3.#Bucket
    ├── k8s/
    │   └── resources.cue          # k8s.#Pod, k8s.#Service, etc.
    └── mypackage/
        └── myresource.cue         # mypackage.#MyResource
```

When adding a schema manually:

```bash
pudl schema add mypackage.my-resource my-schema.cue
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

## Example: Full Custom Schema

Here's a complete example for a custom API resource:

```cue
package myapi

import "list"

// User represents a user account from the internal API
#User: {
    _pudl: {
        schema_type:      "base"
        resource_type:    "myapi.user"
        cascade_priority: 100
        identity_fields:  ["id"]
        tracked_fields:   ["email", "role", "last_login"]
    }

    id:         string & =~"^usr_[a-zA-Z0-9]+$"
    email:      string
    role:       "admin" | "editor" | "viewer"
    last_login: string
    created_at: string

    // Allow extra fields
    ...
}

// AdminUser is a policy schema — a User with admin-specific requirements
#AdminUser: #User & {
    _pudl: {
        schema_type:      "policy"
        base_schema:      "myapi.#User"
        cascade_priority: 120
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
- **Check inference results**: After importing, look at the confidence score. Low confidence (< 0.5) means the schema is a weak match — consider adding more identity fields or adjusting priority.
