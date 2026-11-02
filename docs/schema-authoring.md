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
| `"policy"` | Compliance rules for a type -- evaluated against resources, never inferred or assigned as a record's schema |
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

Paths nest with dots; quote a key that itself contains dots:
`metadata.labels."cloud.googleapis.com/location"`. An identity field that does
not resolve on a record is reported by `pudl import` (and `--dry-run`): such
records fall back to content-hash identity and form no version chain.

### `tracked_fields`

A list of fields to **monitor for changes** between versions of the same resource. These provide a weaker inference signal (+0.1 x ratio of present fields) and are used for diff/change-tracking features.

```cue
tracked_fields: ["State", "InstanceType", "Tags"]
```

### `facts` and `sensitive_fields` (optional)

`facts` declares the relations a schema's records project into the fact store,
so rules, `pudl query` and model checks can use imported fields.
`sensitive_fields` lists paths redacted before anything is stored. Both are
covered in [projection.md](projection.md).

### `base_schema` (optional)

Reference to a parent schema. This builds an inheritance graph used for specificity ordering -- child schemas (more specific) are tried before parent schemas (more generic).
A policy schema also names its type through `base_schema`, but policies are never
tried during classification: only types are assigned. See
[Compliance Policies](#compliance-policies).

### `review` (optional)

`review: {status: "draft" | "validated", by?: string, at?: string}` records
whether a person or agent has reviewed a type schema as a complete
representation of its resource. An absent or invalid block means `draft`; an
invalid one is also a schema load error. `pudl schema list` and `pudl schema
show` display it, and `pudl schema new` writes `status: "draft"`. Policies use
it (see [Absent fields](#absent-fields-and-status)).

## How Validation Works

When data is imported, PUDL determines which schema to assign using a two-phase process:

### Phase 1: Candidate Selection (Heuristic Scoring)

Candidate schemas are scored based on the following (policy schemas are never candidates):
- **Identity field presence**: +0.5 if all `identity_fields` exist in the data
- **Tracked field presence**: +0.1 x (ratio of `tracked_fields` found)
- **Origin keyword matching**: +0.15 if two or more `resource_type` keywords match the origin
- **Schema specificity**: Candidates are sorted most-specific-first using the `base_schema` inheritance graph

### Phase 2: CUE Unification

Starting with the most specific candidate, PUDL attempts CUE unification (`tryUnify`) against the data. The first schema that successfully unifies is assigned.

If no candidate passes unification, PUDL walks the `base_schema` chain -- trying progressively less specific schemas, skipping policies. If the entire chain fails, the catchall schema (`pudl/core.#Item`) is assigned.

This means:
- **No priority numbers** -- ordering comes from the inheritance graph and heuristic scores
- **No explicit fallback lists** -- the `base_schema` chain provides natural fallback
- **Exploratory imports use the catchall**; explicit schema requests are strict unless `--allow-schema-fallback` is supplied.
- **Explicit schemas must be types**: `--schema` or a model's `populate.schema` naming a policy is an error that names the policy's type.

## Compliance Policies

A **type** schema says what a resource is; classification assigns exactly one
type to each record. A **policy** schema (`schema_type: "policy"`) says what a
good instance of a type looks like. Policies are never assigned. After every
import and `#CommandObserve` ingest, PUDL evaluates every policy that applies to
each record's type, and stores one result per resource and policy:
`compliant`, `noncompliant`, or `unknown`, with named violations. A policy
applies when its `base_schema` chain reaches the record's type or one of that
type's ancestors, so several policies can apply to one resource.

Policies cover rules about one resource. Use Datalog rules and checks for
rules that span resources.

### Writing a policy

A policy is a standalone definition that names its type in `base_schema`. Do
not write `#S3BucketBestPractice: #S3Bucket & {...}`: it does not load, because
`_pudl.schema_type` conflicts (`"base"` against `"policy"`) and the closed
definition rejects fields the policy adds. PUDL assembles the chain from
`base_schema` itself.

```cue
package aws

#S3Bucket: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "aws.s3.bucket"
        identity_fields: ["Name"]
        review: {status: "validated", by: "sre-platform", at: "2026-11-01"}
    }
    Name:   string
    Region: string
    PublicAccessBlock?:    {...}
    ServerSideEncryption?: {...}
    ...
}

#S3BucketBestPractice: {
    _pudl: {
        schema_type: "policy"
        base_schema: "aws.#S3Bucket"
        description: "Baseline S3 hardening"
    }
    PublicAccessBlock: {
        BlockPublicAcls:       true @pudl(rule="s3-block-public-acls", severity="high")
        RestrictPublicBuckets: true @pudl(rule="s3-restrict-public", severity="high", doc="Public policies are ignored")
    } @pudl(absent="violation")
    ServerSideEncryption: {
        Algorithm: "aws:kms" | "AES256" @pudl(rule="s3-encryption")
    }
}

// Extends the best practice. The chain must end at a type.
#S3BucketPCI: {
    _pudl: {schema_type: "policy", base_schema: "aws.#S3BucketBestPractice"}
    Versioning: Status: "Enabled" @pudl(rule="s3-versioning", severity="medium")
}
```

A policy constrains only the fields it names. Evaluation ignores closedness at
every depth, so extra record fields never cause a violation and nested policy
structs need no `...`. Policies need no `identity_fields`.

### Rule attributes

`@pudl(...)` on a field names the constraint on it. The keys are:

| Key | Meaning |
|-----|---------|
| `rule` | The rule name reported for a violation at or below this field |
| `severity` | `low`, `medium` (default), `high` or `critical`; requires `rule` |
| `doc` | One-line explanation shown by `pudl policy show`; requires `rule` |
| `absent` | Only `"violation"`; see below. Applies to the field and everything beneath it |

A violation reports the nearest rule at or above the failing path. A failing
constraint with no rule above it reports its path as the rule name, with
severity `medium`. A policy fails to load on an unknown key, an invalid
severity, `severity` or `doc` without `rule`, a rule declared again with a
different severity or doc, or a field with two different rules. Attributes are
checked along the whole chain, so a leftover `@pudl(...)` key on a type field
also breaks that type's policies.

### Absent fields and status

A field the policy requires (not `field?:`) that the record lacks is
**absent**. By default an absence makes the result `unknown`, because the
collector may not have gathered the field. An absence is a **violation** when:

- `@pudl(absent="violation")` is on the field or an ancestor. Use it where the
  source omits a field to mean "off". A missing `PublicAccessBlock` above
  reports `s3-block-public-acls` and `s3-restrict-public`, one per rule
  beneath it.
- The field is required in a type schema of the record's type chain whose
  `review.status` is `validated`. A validated type is a promise that its
  required fields are always collected.

The result is `noncompliant` with any violation, `unknown` with only
absences, and `compliant` otherwise. Each violation has a rule, severity, path
and kind (`conflict` or `absent`); record values are never stored. A result
against a draft type is marked `provisional`.

### Extending policies

A policy whose `base_schema` names another policy extends it. Its result
reports every rule, including inherited ones, so each result stands alone.
Each violation names the policy that declared its rule (`declared_by`). In the
example, a bucket without `PublicAccessBlock` is noncompliant for both
`#S3BucketBestPractice` and `#S3BucketPCI`, and both name
`#S3BucketBestPractice` as the declarer.

### Sensitive paths

Values at `sensitive_fields` paths are redacted before storage, so a policy
cannot evaluate them. A policy that constrains a path the chain declares
sensitive, or a path beneath one, fails to load. A sibling field is fine.

### CUE gotchas

**`&&` and `||` do not short-circuit over absent fields.** `if x == _|_ || x
== false {...}` is undecided when the record lacks `x`, and an undecided
condition applies nothing: the policy silently passes. To treat an absent field
as a value, test the list of present values with `list.Contains`:

```cue
import "list"

#S3NoPublicAcl: {
    _pudl: {schema_type: "policy", base_schema: "aws.#S3Bucket"}
    Archived?: _
    Acl?:      _ @pudl(rule="s3-no-public-acl", severity="high")
    // Unless Archived is true; an absent Archived counts as false.
    if !list.Contains([if Archived != _|_ {Archived}], true) {
        Acl?: !="public-read"
    }
}
```

**Annotate outside comprehensions.** Rules are read from the bare policy, where
a condition that reads record data is undecided. A field annotated only inside
such a comprehension fails to load ("cannot be applied: the field is declared
only inside a comprehension..."). Declare the field with its annotation
outside (`Acl?: _ @pudl(...)` above) and constrain it inside. Fields used in
conditions must be declared too (`Archived?: _`).

**Conditional absence cannot be expressed.** Absences are found from the bare
policy, so a field required only inside a comprehension is never reported as
absent: with `if Website != _|_ {Logging: _}`, a website bucket without
`Logging` is compliant. Instead, put a conflict on a field that is present
whenever the rule matters (an anchor) and name the rule there:

```cue
#S3WebsiteLogging: {
    _pudl: {schema_type: "policy", base_schema: "aws.#S3Bucket"}
    Logging?: _
    // Reported at Website: a website bucket must log access.
    Website?: _ @pudl(rule="s3-website-logging", severity="medium")
    if Logging == _|_ {Website?: "a website bucket needs access logging"}
}
```

`examples/gcp/schema/gcp/policies.cue` uses all three patterns.

### Known limitations

- Pattern constraints (`[string]: T`) are checked for conflicts but not for
  absences; a violation under one takes the nearest named rule above it.
- A constraint that refers to another field the record lacks
  (`size: <=max` without `max`) is neither a conflict nor an absence.
- Conflicts between chain members themselves are not reported at load.

### Policy workflow

```bash
pudl policy list [--type T]              # policies, rule counts, review, result counts, stale
pudl policy show POLICY [--limit N]      # rules, chain, failing resources
pudl policy evaluate [POLICY...]         # re-evaluate stored resources from retained payloads
pudl policy evaluate --dry-run           # report what would change, write nothing
pudl policy evaluate --what-if draft.cue # evaluate an uninstalled draft, write nothing
pudl show ID                             # Compliance: each applicable policy, status, violations
```

A `--what-if` draft must be standalone CUE (no imports of other workspace
packages) and shadows an installed policy of the same name. A policy that
fails to load is reported by these commands and keeps its existing results.

Results are bitemporal facts, `policy_status(resource, policy, type, status,
policy_digest)` and `policy_violation(resource, policy, rule, severity, path,
kind, declared_by)`. Both names are reserved. Checks use the built-in rule
`policy_noncompliant(resource, policy, severity)`: a `#Check` with `query:
"policy_noncompliant"` and `expect: "empty"` fails on any violation, within the
check's evidence selection. `pudl query policy_noncompliant severity=high`
limits it to one severity; for a threshold, write a rule over it. Each result records a digest of the
policy's effective CUE; comments and `_pudl.description` do not change it.
After a policy or its type changes, `pudl policy list` and `pudl doctor`
report stale results. Two doctor repairs cover policies:

```bash
pudl doctor --fix --only policy-results  # re-evaluate stale, orphaned or missing results
pudl doctor --fix --only policy-leaves   # move entries classified to a policy (before
                                         # policies left classification) to its type
```

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
family root is invariant under reinference and specialization refinement
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

Family-root namespacing changes the `resource_id` of entries assigned to a
non-root schema. `pudl doctor` detects entries still carrying a leaf-namespaced
`resource_id`; the identity repair recomputes them and re-sequences the versions
of the resources they join:

```bash
pudl doctor --fix --dry-run --only identity   # preview
pudl doctor --fix --only identity             # apply (idempotent)
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

When generating a schema from a collection entry, use `--collection` to generate a schema for the item type rather than the collection entry:

```bash
pudl schema new --from govim-nupab --collection --path user/#MyItem
```

## Schema File Location

Schemas live in the active workspace's `schema/` directory: `.pudl/schema/`
inside an initialized repository, or `~/.pudl/schema/` in global mode. They are
organized by package, and the schema name format is
`<package-path>.#<Definition>`:

```
<active-pudl-root>/schema/
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

- **Use `user` as your top-level package by default.** Put bespoke schemas under `<active-pudl-root>/schema/user/`, producing names like `user/git.#Repository` or `user/k8s.#CustomResource`. This namespaces all of your schemas together and keeps them clearly separated from PUDL's built-ins.
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

The global schema directory is its own Git repository. Repository-local schemas
are versioned by the enclosing project repository. Use PUDL's built-in commands:

```bash
git status                     # Show uncommitted changes
git commit -m "Add EC2 schema" # Commit
git log                        # View history
```

## After Adding or Changing Schemas

Run `pudl doctor` to find existing catalog entries whose inference now selects a
different schema, and the inference repair to re-classify them:

```bash
pudl doctor --fix --only inference
```

This updates the schema assignments of inferred entries. Add
`--include-explicit` (optionally with `--schema` or `--origin`) to re-infer
explicit assignments too. Entries that previously fell through to the catchall may now match your new, more specific schema.

You can also verify that inference is stable after changes:

```bash
pudl doctor
```

This confirms that re-running inference on every entry produces the same schema assignment it already has.

## Example: Full Custom Schema

Here is a complete example for a custom API resource:

```cue
package myapi

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

// A policy evaluated against every User; never assigned as a record's schema
#UserBestPractice: {
    _pudl: {
        schema_type: "policy"
        base_schema: "myapi.#User"
        description: "Every user has MFA"
    }

    mfa_enabled: true @pudl(rule="user-mfa", severity="high", absent="violation")
}
```

After adding this file:

```bash
pudl schema add myapi.users users.cue
git commit -m "Add user schemas"
pudl doctor --fix --only inference  # Re-classify existing imports
```

## Tips

- **Start permissive, tighten later**: Use `...` to accept extra fields, then constrain as you learn your data shape
- **Identity fields matter most**: Good identity fields enable version tracking and strong inference. Choose fields that uniquely identify a resource instance.
- **Run `pudl doctor --fix --only inference`** after any schema change to re-classify existing data
- **Check inference results**: After importing, look at the confidence score. Low confidence (< 0.5) means the schema is a weak match -- consider adding more identity fields or refining the schema structure.
- **Use `base_schema` for inheritance**: Policy schemas reference their type via `base_schema`; the chain must end at a type. Classification and validation fallback skip policies.
- **Try a policy before installing it**: `pudl policy evaluate --what-if draft.cue` evaluates it against stored resources and writes nothing.
