# Shared identity core (2026-11-05)

Issue: `pudl-h7s.1` (epic `pudl-h7s`). Design:
`docs/design/2026-11-05-shared-identity-across-schemas.md` §1–3.

Records of different schemas that describe one real resource can now be
linked, without merging them. A schema declares the keys its records hold;
projection writes them as facts; a built-in rule links resources with equal
keys; a pure package groups the links.

## Public API

### Schema metadata: `_pudl.same_resource`

```cue
_pudl: same_resource: [
	{kind: "k8s.cluster", key: {host: "endpoint"}},
	{kind: "k8s.cluster", key: {host: {path: "server", trim_prefix: "https://"}}},
]
```

- A non-empty list of `{kind, key}`. `kind` is dotted lower-case words
  (`k8s.cluster`). `key` maps identifier part names to a field path or
  `{path, trim_prefix?, last_segment?, prepend?}` (graphspec's syntax).
- At most one part per declaration may contain `[*]`; it yields one key per
  element. A missing, empty or non-scalar part yields no key.
- Strictly decoded (`validator.decodeSameResource`). An invalid block is a
  schema load error (`SchemaMetadata.SameResourceError`) and disables
  projection for the schema, like `_pudl.owner`.
- Inherited along `base_schema`; the nearest declaration wins. Needs
  `identity_fields`.

### Relations

| Relation | Args | Written by |
| --- | --- | --- |
| `same_resource_key` | `entry_id, resource_id, kind, key` | Projection only (reserved); `key` is the canonical JSON of the parts |
| `same_resource` | `a, b, kind` | Rules only (reserved): built-in `same_resource_by_key`, and pack rules |
| `same_resource_kind` | `kind, label, meaning` | Ground facts: built-ins declare `k8s.cluster` and `k8s.object` |

`same_resource` includes `a = b` rows; consumers drop them.

Built-in rule file: `internal/importer/bootstrap/pudl/rules/same_resource.cue`.

### Go

- `internal/sameresource` (pure):
  - `Parse(raw json.RawMessage) (*Spec, error)`; `(*Spec).Keys(record) []Key`;
    `(*Spec).Canonical() string`.
  - `Build(links []Link, namespaceOf func(string) string) ([]Group, []Conflict)`:
    connected components. A group's ID is its smallest member. A component
    with two members of one identity namespace is returned as a `Conflict`
    and not formed. An empty namespace never conflicts.
  - Constants `LinkRelation`, `KeyRelation`, `KindRelation`, `ArgKind`, `ArgKey`.
- `internal/database`: `SameResourceRelation`, `SameResourceKeyRelation`;
  `IsReservedRelation` covers both. `AddFact` rejects them, except
  `same_resource_key` under a projection source.
- `internal/projection`: `SchemaSpec.SameResource`; `Compute` emits the key
  facts; the fingerprint includes the spec (omitted when unset, so existing
  fingerprints do not change); `RelationArgs` and `RelationSchemas` cover
  `same_resource_key`.
- `internal/graphspec`: `ParseValue(raw, wildcards bool)` and
  `Value.Apply(raw)` exported for reuse.

## Tests

- `internal/sameresource`: parsing mistakes, keys from several declarations,
  missing parts, wildcard fan-out, grouping, conflicts.
- `internal/validator/same_resource_test.go`: decode and load errors.
- `internal/database/same_resource_relations_test.go`: reservation rules.
- `internal/projection/same_resource_test.go`: inheritance, broken
  declarations, fingerprint, and end to end (two schemas' records linked by
  the built-in rule through `datalog.Evaluate`, a third left out).
- Smoke: `pudl query same_resource_kind --global` lists both kinds; `pudl
  doctor` reports nothing new.

## Not in this step

Graph export groups, `pudl show`, doctor checks (undeclared kinds,
conflicts) and docs are `pudl-h7s.2`.
