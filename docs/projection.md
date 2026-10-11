# Querying imported data: fact projection and sensitive fields

Imported records live in the catalog as payload files. Datalog rules,
`pudl query` and model `#Check`s read **facts**. A schema bridges the two
by declaring which of its records' fields become facts. It can also declare
fields that must never be stored at all.

Design and review history: `docs/design/2026-10-08-gcp-cataloging-ux.md`.

## Declaring facts (`_pudl.facts`)

```cue
package gcp

#Firewall: {
	_pudl: {
		schema_type:     "base"
		resource_type:   "gcp.firewall"
		identity_fields: ["project", "name"]          // required with facts
		facts: {
			gcp_firewall: args: {
				project: "project", name: "name", direction: "direction"
				disabled: {path: "disabled", default: false}
				network:  {path: "network", trim_prefix: "https://www.googleapis.com/compute/v1/"}
			}
			gcp_firewall_source: args: {range: "sourceRanges[*]"}
			gcp_firewall_allow: {
				each: "allowed[*]"                     // one fact per element
				args: {proto: "IPProtocol", port: {path: "ports[*]", default: "*"}}
			}
		}
	}
	kind: "compute#firewall"
	...
}
```

Each relation is `{each?: path, args: {name: argspec}}`. An argspec takes one
of three forms:

| Form | Value |
|---|---|
| `"path"` | the field at path; the arg is omitted when the field is absent |
| `{path, default?, trim_prefix?}` | the field, or `default` when it is absent; a string prefix stripped |
| `{exists: path}` | `true` if the path is present, else `false` |

Rules for args:

- **`each`** turns every element at a wildcard path into a row, so its arg
  paths are relative to that element. Fields from the whole record are reached
  by joining on `entry_id` (below).
- **One wildcard per relation.** At most one arg may contain `[*]`; it yields
  one fact per value. Use `each` to keep values paired, for example protocol
  with port.
- **Implicit args.** Every fact gets `entry_id` (the catalog entry) and
  `resource_id`. To join a relation to the catalog, use
  `catalog_entry(id: $E)` with `entry_id: $E`.
- **Values.** Numbers keep their exact text. A value the query layer cannot
  represent, such as an integer beyond int64, is left out and reported.
- **Inheritance.** Relations are inherited along `base_schema`; when names
  collide, the nearer schema wins.

### Field paths

All `_pudl` paths use one syntax: identity fields, facts, sensitive fields, and
`pudl import --set`.

- **Dots nest:** `metadata.name`.
- **Quotes allow keys that contain dots:** `metadata.labels."cloud.googleapis.com/location"`.
- **`[*]` fans out over an array** (not allowed in identity fields).

### What stays current

A resource's facts always describe its **most recently observed** entry,
whether that entry came from `pudl import` or a model run. That includes
re-importing content that is already cataloged, so a resource that changes and
then changes back reports its current state.

When a later observation replaces facts, the old facts are invalidated: the
valid-time history shows the change. Corrections are different and the old
facts are retracted. These are:

- a changed facts block
- an entry that was deleted, pruned or re-identified
- a record reassigned to another schema

Projection needs identity. Records whose identity fields do not resolve are not
projected, and the import says so: without identity, every state of a resource
would be its own resource and its facts would never go away.

A resource missing from a later import keeps its facts; projection does not
detect deletions.

### Keeping facts in sync

`pudl import` and `pudl run` project as records arrive. Afterwards they bring
everything else up to date: data imported before a facts block existed,
changed facts blocks, and deleted or reassigned entries. `schema reinfer`,
`delete`, `snapshot prune` and `migrate identity` do the same when they finish.

`pudl facts reproject [--dry-run]` runs this explicitly. `pudl query` writes
nothing, but warns when a reprojection is pending.

A facts block that is invalid disables projection for its schema. Existing
facts are left unchanged and every import, run and query says so. Invalid
blocks include:

- a relation or arg name that is not an identifier
- a reserved relation name
- two wildcard args in one relation
- `each` without a wildcard
- facts declared without `identity_fields`

### Silent misses are reported

Checks carry `outcome: pass|fail|unknown|error`. A missing relation, invalid
required projection, or failed required synchronization produces `unknown`,
never a pass. Evaluation failures produce `error`. Both are persisted with
structured diagnostics and exit 1 under `--detailed-exitcode`. A declared empty
relation remains valid; unrelated broken schemas do not disable a check.

PUDL also reports potential projection and rule misses during exploration:

- **Import and run summaries** list the facts projected per relation. They warn
  when a declared relation produced none (usually a path typo) and when values
  were left out.
- **`pudl query` and `pudl run`** lint the rules a query or check depends on:
  - a body relation that nothing produces (no rule, no facts block, no stored
    facts)
  - an arg that the relation's facts block does not declare
  - a projected relation that is also a rule head

Projected facts carry no `run_id`, so checks over them evaluate current state
across the whole catalog.

### Writing rules over projected facts

```cue
package rules

open_ssh: {
	head: {rel: "open_ssh", args: {project: "$P", name: "$N"}}
	body: [
		{rel: "gcp_firewall", args: {entry_id: "$E", project: "$P", name: "$N", direction: "INGRESS", disabled: false}},
		{rel: "gcp_firewall_source", args: {entry_id: "$E", range: "0.0.0.0/0"}},
		{rel: "gcp_firewall_allow", args: {entry_id: "$E", proto: "tcp", port: "22"}},
	]
}
```

From the command line, booleans and numbers are typed:
`pudl query gcp_firewall disabled=false`. Use `name='"123"'` to force a string.

The datalog has no negation. To express "has no X", project `{exists: "X"}` or
give a `default`, then match on that value.

## Sensitive fields (`_pudl.sensitive_fields`)

```cue
_pudl: sensitive_fields: ["spec.template.spec.containers[*].env[*].value"]
```

Each matching value is replaced with `"[REDACTED]"` before the record is
hashed, identified, stored or projected. This applies to `pudl import` and to
model runs alike. Fields are inherited along `base_schema`.

Redaction **fails closed**. The record is not stored when any of these hold:

- `sensitive_fields` is not a list of valid paths
- an unknown key such as `sensitive_mode` is present
- a sensitive path overlaps an identity field
- the record is a YAML or CSV document routed to a schema with sensitive fields

The paths come from the record's **intended** schema (`pudl import --schema`,
or a model's `populate.schema`) and its **assigned** schema. A record that
fails validation and falls back to a base or catchall schema is still
redacted.

Limits:

- **Routing.** Redaction depends on routing. A record that inference assigns to
  some other schema, with no explicit schema, gets that schema's sensitive
  fields. Route sensitive data explicitly.
- **Temp files.** Decompressed, envelope and stdin temporary files can hold
  plaintext while an import runs.
- **Retroactive.** Payloads stored before a field was declared sensitive are
  not rewritten. Projection still redacts them in memory.
- **Drift.** Redacted values never equal plaintext `desired` values, so do not
  declare desired state on sensitive paths.

Projection records an incomplete status when values fall outside the query
numeric domain. Dependent latest-known checks become unknown, just as selected
snapshot checks do. Projection contract version 2 refreshes older projection
state on the next synchronization; source payloads and fact history are retained.
