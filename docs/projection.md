# Projection: facts, sensitive fields and secret warnings

Imported and observed records live in the catalog as payload files. Datalog
rules, `pudl query` and model checks read **facts**. A type schema bridges the
two by declaring which of its records' fields become facts (`_pudl.facts`). It
can also declare fields that must never be stored (`_pudl.sensitive_fields`)
and paths the secret scan should not report (`_pudl.secret_scan_ignore`).

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
- **One or many.** Some documents write a single item without its list. AWS
  policy JSON does this for a lone statement, and for a lone principal or
  action. With `one_or_many: true` on a relation, every `[*]` in it (in
  `each` and in args) treats a value that is not a list as a one-element
  list:

  ```cue
  aws_role_trust: {
  	each:        "AssumeRolePolicyDocument.Statement[*]"
  	one_or_many: true
  	args: {effect: "Effect", principal: "Principal.AWS[*]"}
  }
  ```

  Without it, `[*]` over a non-list yields nothing, as before.
- **Booleans come back as numbers.** The query layer returns a stored `true`
  as `1`. Match it in a rule body with the constant `true`, but do not expect
  a boolean in rule output.
- **Implicit args.** Every fact gets `entry_id` (the catalog entry) and
  `resource_id`. To join a relation to the catalog, use
  `catalog_entry(id: $E)` with `entry_id: $E`.
- **Values.** Numbers keep their exact text. A value the query layer cannot
  represent, such as an integer beyond int64, is left out and reported, and
  checks that read that relation are `unknown`.
- **Inheritance.** Relations are inherited along `base_schema`; when names
  collide, the nearer schema wins.

### Field paths

All `_pudl` paths use one syntax: identity fields, facts, sensitive fields, and
`pudl import --set`.

- **Dots nest:** `metadata.name`.
- **Quotes allow keys that contain dots:** `metadata.labels."cloud.googleapis.com/location"`.
- **`[*]` fans out over an array** (not allowed in identity fields).
- **`[*!field=["a","b"]]` fans out over every element except** objects whose
  `field` holds one of the listed strings:
  `metadata.items[*!key=["enable-oslogin"]].value`. An element without the
  field, with a non-string value there, or that is not an object is not
  skipped. `[*!` always starts a filter; a malformed one is an error.

### What stays current

A resource's facts always describe its **most recently observed** entry,
whether that entry came from `pudl import` or a `pudl check` observation. That includes
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

`pudl import` and `pudl check` project as records arrive. Afterwards they bring
everything else up to date: data imported before a facts block existed,
changed facts blocks, and deleted or reassigned entries. `delete`,
`snapshot prune` and the `pudl doctor --fix` repairs do the same when they finish.

`pudl doctor` reports pending reprojection, and
`pudl doctor --fix [--dry-run] --only projections` runs it explicitly. `pudl query` writes
nothing, but warns when a reprojection is pending.

A facts block that is invalid disables projection for its schema. Existing
facts are left unchanged and every import, check and query says so. Invalid
blocks include:

- a relation or arg name that is not an identifier
- a reserved relation name
- two wildcard args in one relation
- `each` without a wildcard
- facts declared without `identity_fields`

### Silent misses are reported

A check whose relation nothing produces, or whose projection is invalid or
failed to sync, is `unknown`, never a pass. A declared relation that is simply
empty is valid, and an unrelated broken schema does not disable a check.

pudl also reports likely misses while you explore:

- **Import and check summaries** list the facts projected per relation. They warn
  when a declared relation produced none (usually a path typo) and when values
  were left out.
- **`pudl query` and `pudl check`** lint the rules a query or check depends on:
  - a body relation that nothing produces (no rule, no facts block, no stored
    facts)
  - an arg that the relation's facts block does not declare
  - a projected relation that is also a rule head

Projected facts carry no `run_id`. A check without `evidence` reads them
across the whole catalog; a check with `evidence` rebuilds them from the
selected snapshots (see [observation](observation-scope.md#select-evidence)).

### Writing rules over projected facts

Join a resource's relations on `entry_id`; [Datalog](datalog.md#write-a-rule)
has a rule over the firewall relations above. Datalog has no negation: to
express "has no X", project `{exists: "X"}` or give a `default`, then match
on that value.

## Resource ownership

A schema that declares `_pudl.owner` (see
[schema authoring](schema-authoring.md#owner)) projects one more
fact per resource, kept current exactly like its facts block:

- `resource_owner(entry_id, resource_id, type, owner, via)` when a path, the
  mapping table or the fallback names an owner. `via` is `path:<path>`,
  `map:<path>` or `fallback`.
- `resource_unowned(entry_id, resource_id, type)` otherwise.

Because unowned is a positive fact, a check can list unowned resources without
negation:

```cue
{name: "unowned", query: "resource_unowned", expect: "empty", severity: "warn",
 message: "Label these resources with a team or map their project"}
```

and a rule can join any resource-keyed relation to its owner:

```cue
violation_owner: {
	head: {rel: "violation_owner", args: {resource: "$R", policy: "$P", owner: "$O"}}
	body: [
		{rel: "policy_violation", args: {resource: "$R", policy: "$P"}},
		{rel: "resource_owner", args: {resource_id: "$R", owner: "$O"}},
	]
}
```

A facts block cannot declare these two relation names. Changing a type's
`owner` block or its mapping table re-derives every owner on the next sync.
How check findings are attributed to owners is described in
[governance](governance.md#attribute-findings-to-owners).

## Sensitive fields (`_pudl.sensitive_fields`)

```cue
_pudl: sensitive_fields: ["spec.template.spec.containers[*].env[*].value"]
```

Each matching value is replaced with `"[REDACTED]"` before the record is
hashed, identified, stored or projected. This applies to `pudl import` and to
model observations alike. Fields are inherited along `base_schema`.

### Allowlists

An entry can instead be a struct that redacts a value in every element of an
array **except** the elements named on a list:

```cue
_pudl: sensitive_fields: [{
	each:  "metadata.items[*]"   // the array; must end in [*]
	value: "value"               // the path redacted in each element; omit for the element itself
	keep_when: {field: "key", in: _safeMetadataKeys}
}]
_safeMetadataKeys: ["enable-oslogin", "serial-port-enable"]
```

Items whose `key` is listed keep their values; every other value is redacted,
including keys added after the list was written. The struct is the same as
the path `metadata.items[*!key=["enable-oslogin","serial-port-enable"]].value`
(see [field paths](#field-paths)), which is how pudl reports it. Likely-secret
warnings still scan the values an allowlist keeps.

Redaction **fails closed**. The record is not stored when any of these hold:

- `sensitive_fields` is not a list of valid paths and allowlist structs (an
  allowlist needs `each`, `keep_when.field` and `keep_when.in`, and unknown
  keys are errors)
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
  not rewritten on import. Projection still redacts them in memory, and
  `pudl doctor --fix --only sensitive-scrub` rewrites them (see
  [doctor](cli-reference.md#pudl-doctor)).
- **Drift.** Redacted values never equal plaintext `desired` values, so do not
  declare desired state on sensitive paths.

## Likely-secret warnings (`_pudl.secret_scan_ignore`)

Redaction only covers what a schema declares. After schema assignment and
redaction, `pudl import` scans every record for likely secrets elsewhere: AWS
access key IDs and secret keys, GCP service-account keys and API keys, GitHub,
GitLab and Slack tokens, PEM private keys, JWTs, URLs with `user:password@`,
and long random-looking strings. Each finding is reported by record, path and
kind, never by value — under `--dry-run`, in the import summary and as
`secret_warnings` (plus `secret_warnings_total`) in `--json`. Nothing is
rewritten: the record, its hash and its storage are unchanged.

Resolve a finding by adding the path to `sensitive_fields`, which redacts it,
or, if the value is not a secret, by listing it in `secret_scan_ignore`:

```cue
_pudl: secret_scan_ignore: ["spec.nonce", "status.conditions[*].token"]
```

A listed path covers itself and everything below it. The list is inherited
along `base_schema`. A list that is not concrete, not a list of strings, or
holds an unparsable path is a schema load error and suppresses nothing.

`pudl doctor` scans stored payloads and current facts the same way. The
advanced `pudl facts add` warns on its args; facts are never redacted.
`pkg/factstore` callers can opt in to rejecting such facts with
`Open(dir, factstore.WithSecretRejection())`.
