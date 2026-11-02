# Ownership attribution for resources and findings (pudl-xf5.6)

Schemas declare how a resource's owner is derived. Ownership is projected at
ingest as two built-in relations, check findings and policy results name the
owner, `pudl check --owner TEAM` and `pudl policy show --owner TEAM` show one
owner's findings, and unowned resources are a queryable relation.

## Declaration surface

```cue
_projectOwners: {"shop-prod": "payments"}   // hidden package value, shared

#Bucket: _pudl: owner: {
	paths: ["labels.team", "tags.Owner"]          // tried in order
	map: {path: "project", owners: _projectOwners} // value -> owner
	fallback: "platform"                           // optional
}
```

Resolution order: the first path holding a non-empty string, then the mapping
table, then the fallback; otherwise unowned. The whole declaration is
inherited along `base_schema` (nearest wins). Decoding is strict
(`internal/validator/owner.go`): unknown keys, `[*]` paths, empty owners, a
missing `map.owners`, or a block with none of the three are schema load errors
(`CheckModuleIntegrity`) and disable projection for the schema.

Why this surface: one optional block covers labels and tags (paths), a
project or account table (map) and a fallback, with nothing else. The table is
declared in the schema tree rather than in a separate workspace or model file
because ownership must be known when a record is ingested, whichever model
observed it; a model-level table would make a resource's owner depend on the
check that reads it. Sharing comes from CUE itself: the table is one hidden
package value referenced by every type (the GCP pack does exactly that in
`schema/gcp/owners.cue`). Living in `_pudl` gives strict decoding,
`base_schema` inheritance and projection fingerprinting for free, so editing
the table re-derives every owner on the next sync.

## Attribution flow

1. `validator` decodes `_pudl.owner` into `SchemaMetadata.Owner`
   (`*ownership.Spec`) or `OwnerError`.
2. `projection.Registry` resolves the nearest declaration along the chain into
   `SchemaSpec.Owner`; it is part of the spec fingerprint. A schema that
   declares an owner but no facts now projects. `Compute` emits one fact per
   record: `resource_owner(entry_id, resource_id, type, owner, via)` or
   `resource_unowned(entry_id, resource_id, type)`. Import, observation, the
   check-time sync, snapshot evidence views and `doctor --fix --only
   projections` all use this path unchanged. A facts block cannot declare the
   two names. They are not database-reserved: evidence views copy projected
   facts with `AddFact`, and policy relations' reservation would make a check
   over `resource_unowned` pass silently when nothing declares an owner; as
   projected relations they get the usual missing-relation diagnostic.
3. `operation.runChecks` loads `ownership.Index` (current ownership facts) and
   `checkreport.AttributeOwners` sets `Witness.Owner` from the witness's
   `resource_id`/`resource` argument, then its entry IDs (an older entry
   resolves to the resource's current owner). Report JSON carries `owner` on
   every witness (`""` when none).
4. Human output (`ModelReport.Markdown`) groups a failed check's findings
   under `owner X:` headings, owners in name order and `(none)` last; a check
   whose findings have no owner prints the flat list as before.
5. `pudl check --owner TEAM` filters what is printed: live member reports
   (`Output.Owner` -> `ModelReport.ForOwner`) and the JSON check report
   (`checkreport.CheckForOwner`). A failed check without a TEAM finding is
   left out; passed and uncertain checks, drift, counts, the summary, the
   recorded report and the exit status are unchanged.
6. `pudl policy show` adds `owner` to every failing resource (JSON and text)
   and `--owner TEAM` keeps only TEAM's.

Unowned resources: `resource_unowned` is a positive fact, so a model check
`{query: "resource_unowned", expect: "empty"}` lists them (its witnesses carry
`entry_id`, `resource_id` and `type`), and `pudl query resource_unowned`
prints them. Only types that declare (or inherit) an owner are covered.

Limits: attribution covers recorded witnesses (at most 20 per check). A
witness keyed only by another identifier (the GCP pack's selfLink-keyed
cross-resource rules) has no owner.

## Public API

`internal/ownership` (new)
- `Spec{Paths, Map, Fallback}`, `Mapping{Path, Owners}`, `Parse(raw) (*Spec,
  error)`, `(*Spec).Resolve(record) (owner, via, ok)`, `(*Spec).Canonical()`.
- `ViaPathPrefix`, `ViaMapPrefix`, `ViaFallback`; `OwnerRelation`
  (`resource_owner`), `UnownedRelation` (`resource_unowned`).
- `Attribution`, `Index`, `Load(db)`, `(*Index).Resource(id)`,
  `(*Index).Entry(id)`, `(*Index).OwnerOf(resourceIDs, entryIDs)`.

`internal/validator`: `SchemaMetadata.Owner`, `SchemaMetadata.OwnerError`.

`internal/projection`: `SchemaSpec.Owner`; `ArgType`, `ArgOwner`, `ArgVia`.

`internal/checkreport`: `Witness.Owner`, `OwnerOf`, `AttributeOwners(results,
ownerOf)`, `(*ModelReport).ForOwner(owner)`, `CheckForOwner(report, owner)`.

`internal/operation`: `Output.Owner`.

CLI: `pudl check --owner TEAM`; `pudl policy show POLICY --owner TEAM`;
`failing[].owner` in `pudl policy show --json`; `witnesses[].owner` in check
and report JSON.

GCP pack: `schema/gcp/owners.cue` (`_gcpProjectOwners`, `_#GCPOwner`); every
type declares `owner`, with its team label path where it has labels
(`labels.team`, `resourceLabels.team`, `settings.userLabels.team`,
`metadata.labels.team`). `policies.cue` is untouched.

## Tests

- `internal/ownership`: invalid blocks rejected; resolution order (first path,
  second path, empty and non-string labels skipped, map, fallback, unowned);
  stable canonical form.
- `internal/validator`: valid decode with a shared package table; unknown
  key, non-concrete, wildcard and empty blocks are load errors.
- `internal/projection`: owner inherited along `base_schema`; labeled, mapped
  and unowned facts; nearest declaration wins; a broken parent declaration
  disables projection; owner without identity; a mapping change reprojects
  owners through `Sync`; facts blocks cannot declare the names.
- `internal/checkreport`: attribution by resource then entry; grouping;
  `ForOwner`; `CheckForOwner` leaves the recorded report unchanged.
- `cmd/ownership_e2e_test.go`: a fleet fixture with labeled, project-mapped
  and unlabeled servers plus an inheriting GPU type gives the right owners per
  finding, human grouping, `--owner` filtering (JSON and text; exit status
  unchanged), the unowned list (check and `pudl query`), `via` values,
  `policy show` owners and `--owner`; invalid owner metadata reported by
  `pudl doctor`; the installed GCP pack attributes the fixture Pub/Sub topics
  to `orders` and lists unlabeled buckets as unowned.

## Line deltas against sre-focus (8dcd306, after merging it)

`git diff --numstat sre-focus`, Go files:

| | Added | Removed | Net |
| --- | --- | --- | --- |
| Production | +661 | −24 | +637 |
| Tests | +571 | −0 | +571 |

The merge with the waivers work (`pudl-xf5.4`) put both `Witness.Waiver` and
`Witness.Owner` on findings; grouped human output keeps each finding's waiver
note.

## Verification

`CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 mise exec -- make lint` (0
issues), `mise exec -- make check-docs check-skills`.
