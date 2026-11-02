# Policy evaluation engine (`internal/policy`) — pudl-xf5.8.2

Delivery step 2 of `docs/design/2026-11-01-compliance-policies.md`. A pure
package: it reads loaded CUE schemas and metadata and a record
(`map[string]any`), and never touches the database. Nothing calls it yet:
persistence, `pudl policy` commands and ingest wiring are `pudl-xf5.8.3`/`.8.4`.

## Public API

```go
// Source: the loaded schema set. *inference.SchemaInferrer satisfies it.
type Source interface {
    redact.Source // GetAvailableSchemas, GetSchemaMetadata, GetInheritanceGraph
    GetSchemaValue(name string) (cue.Value, bool)
}

func Load(src Source) (*Set, []LoadError)
func (s *Set) Policies() []*Policy                 // sorted by name
func (s *Set) Policy(name string) (*Policy, bool)  // name is normalized
func (s *Set) Applicable(typeName string) []*Policy
func (s *Set) Evaluate(p *Policy, typeName string, record map[string]any) (*Result, error)

type LoadError struct{ Policy string; Err error }  // Error() string

type Policy struct {
    Name    string
    Type    string   // the type schema the chain ends at
    Extends string   // the policy it extends, or ""
    Chain   []string // base_schema chain, policy first, root type last
    Digest  string   // "sha256:<hex>" of the effective CUE
    Rules   []Rule   // inherited rules included, sorted by name
}
type Rule struct {
    Name string; Severity Severity; Doc string
    Paths []string; DeclaredBy string
}
type Result struct {
    Policy, Type, Digest string
    Status      Status      // compliant | noncompliant | unknown
    Violations  []Violation // make the result noncompliant
    Unknown     []Violation // absent, not violations; make a clean result unknown
    Provisional bool        // the record's type is not review-validated
}
func (r *Result) HighestSeverity() Severity       // derived, "" when none
type Violation struct {
    Rule string; Severity Severity; Path string
    Kind Kind          // conflict | absent
    DeclaredBy string  // JSON "declared_by_policy"
}

const SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical Severity
const DefaultSeverity = SeverityMedium
const StatusCompliant, StatusNoncompliant, StatusUnknown Status
const KindConflict, KindAbsent Kind
const SchemaTypePolicy = "policy"
```

Violations never carry record values; paths use `internal/fieldpath` syntax,
with list elements written `[*]`.

`internal/fieldpath` gains `func (p Path) HasPrefix(prefix Path) bool`, used by
the sensitive-path check.

## Authoring: policies are standalone, linked by `base_schema`

The design's example writes `#S3BucketBestPractice: #S3Bucket & {_pudl: ...}`.
**That does not load.** The type's `_pudl.schema_type: "base"` and the
policy's `"policy"` conflict, so the whole CUE package fails to build
(`#BP._pudl.schema_type: conflicting values "base" and "policy"`). The same
applies to `#S3BucketPCI: #S3BucketBestPractice & {...}`, and also to the
existing example in `docs/schema-authoring.md`. In addition, `#Def & {new: ...}`
rejects every field that `#Def` does not declare ("field not allowed").

So a policy is written standalone and names its base in metadata:

```cue
#S3BucketBestPractice: {
    _pudl: {schema_type: "policy", base_schema: "aws.#S3Bucket"}
    PublicAccessBlock: {
        BlockPublicAcls: true @pudl(rule="s3-block-public-acls", severity="high")
    } @pudl(absent="violation")
}
```

The engine composes the effective policy from the `base_schema` chain
(`InheritanceGraph.GetCascadeChain`): the policy, any policies it extends, and
the type and its ancestors. `pudl-xf5.8.9` (docs) must use this form.

## How evaluation works

**Loading.** `Load` takes every schema with `schema_type: "policy"`. For each
one it:

1. Walks the chain to the first non-policy schema, which is the type. A
   policy without `base_schema`, or whose chain never reaches a type, fails.
2. Builds one field tree merged across the chain, most-base schema first.
   Each node records whether the field is required, the `@pudl` rule and
   `absent` flag on it, which schema first required it, and whether a policy
   (not the type) declares it. Lists contribute a `[*]` node through
   `LookupPath(cue.AnyIndex)`.
3. Validates attributes strictly. Allowed keys are `rule`, `severity`, `doc`
   and `absent`. Severity must be low, medium, high or critical. `absent` may
   only be `"violation"`. `severity` and `doc` require a `rule`. A rule name
   declared again with a different severity or doc fails, and so does a field
   annotated with two different rules. A rule without a severity defaults to
   `medium`.
4. Rejects a policy that constrains a sensitive path. It takes
   `redact.NewRegistry(src).For(policy)` (the union of `sensitive_fields` along
   the chain) and fails if any policy-declared field equals or lies beneath a
   sensitive path. A sibling such as `Credentials.KeyId` next to a sensitive
   `Credentials.Secret` is allowed.
5. Computes the digest. For each chain member it exports `Value.Syntax`
   without docs, with attributes and optional fields, deletes the `_pudl` field
   using `astutil`, and formats the result. The digest is the SHA-256 of the
   member name followed by that output, over every member. Comments and
   `_pudl` text such as `description` therefore do not change the digest.
   Constraints, rule attributes and chain membership do, including a change in
   a base policy or the type.

Policies that fail are returned as `LoadError` and left out of the `Set`.

**Applicability.** A policy applies to type T when the policy's type is in T's
cascade chain, so a policy on `#S3Bucket` also applies to a custom
`#S3BucketSpecial` that extends it. Nothing applies to a type that is itself a
policy.

### Open evaluation: the CUE approach

Approaches tried against CUE v0.16.0:

- `Value.UnifyAccept(data, accept)` with `accept` set to the data, `{...}`, or
  the unified value. The closedness errors stayed.
- Pruning the record with `Value.Allows(sel)`. This removes extra record
  fields, but it cannot ignore the closedness that the policy itself
  triggers, such as a field the policy adds beyond its base.
- `#Def...` (the `explicitopen` experiment). It opens only the top level,
  needs a per-file `@experiment`, and cannot reach list-element structs.

The approach used: the record is unified with **each chain member
separately** (`member.Unify(data).Validate()`, with data compiled once per CUE
context), and errors are filtered:

- Errors whose message format is exactly `"field not allowed"` are dropped.
  This is CUE's single closedness error (`internal/core/adt/fields.go`), so
  extra record fields, extra list-element fields and policy-added fields
  never count.
- Errors under hidden fields or definitions (an unquoted `_` or `#` label)
  are dropped.
- Errors at a path the record does not have are dropped. Absences are found
  by the walk, not by CUE.

Validation is **not** concrete. A constraint the record leaves incomplete is
an absence, not a conflict.

For a concrete record, unifying with each member separately finds the same
data conflicts as unifying the whole chain. It also works when the chain spans
a repository path and the global path, which are separate CUE runtimes and
cannot be unified with each other.

### Mapping errors to `@pudl` attributes

`cue/errors.Errors(err)` gives structured errors, and `Error.Path()` gives
label strings. CUE prints identifiers bare and other keys JSON-quoted, so
`"_id"` and `"a.b"` stay distinguishable from hidden fields. It prints list
indices as digits. `parseErrorPath` decodes these labels, and `resolveInRecord`
follows them through the record. That check confirms the record has the path
and turns indices into `[*]`.

Next, the merged tree is walked down that path, keeping the nearest
`@pudl(rule=...)` at or above it. A matching rule gives its name, severity and
declarer. Otherwise the path itself is the rule, with the default severity,
attributed to the first chain member (most-base first) whose unification
rejected the value. A conflict the type already rejects is therefore
attributed to the type.

`internal/validator/cue_error_parser.go` was not reused. It regex-parses
error strings into `Expected`/`Got` values, but violations must not carry
values, and the structured path is more reliable than string parsing.

### Absent-field semantics

The tree is walked against the record. A required field that the record lacks
is absent. The design names the nearest rule at or above the failing path. If
the missing field has no such rule, each annotated required field beneath it
is reported under its own rule, so a missing `PublicAccessBlock` yields
`s3-block-public-acls` and `s3-restrict-public`, and waivers keyed by rule keep
working. The field itself is also reported when an unannotated required leaf
lies beneath it, or when nothing beneath it is annotated.

An absence is a **violation** when either of these holds:

- `@pudl(absent="violation")` is on the field or any ancestor.
- The missing field is required at that path in a **validated** type schema
  of the record's type chain. This includes a validated ancestor of a draft
  custom type.

Otherwise the absence goes in `Unknown`.

`Provisional` is true when the record's own type is not validated.

Status is `noncompliant` with any violation, `unknown` with only unknowns, and
`compliant` otherwise. Findings are deduplicated by (rule, path, kind) and
sorted.

## Tests (`internal/policy`, table-driven)

The schemas are loaded through the real `inference.NewSchemaInferrer` from a
temporary CUE module. The tests cover:

- compliant records, and extra fields at the top level, in nested structs, in
  list elements and next to constrained fields;
- a conflict on an annotated rule, and on a struct-typed field;
- unannotated constraints (path as rule), and attribution of conflicts to the
  policy or the type;
- an absence without the opt-in (unknown, also when nested), with the opt-in
  (one violation per rule), and with the opt-in inherited by a nested field;
- a field required in a validated type, and in a validated ancestor of a
  custom type;
- a field required in a draft type (unknown) and the provisional flag;
- a policy chain whose inherited rules carry their declarer;
- conflicts and absences inside list elements;
- a mix of a violation and an unknown, which is noncompliant;
- `HighestSeverity`, and violations never containing values;
- applicability (descendants yes, policies no) and rejection of unrelated
  types;
- the policy's shape (type, extends, chain and rule declarers);
- load errors: invalid severity, unknown key, invalid `absent`, severity
  without a rule, a rule redeclared with a different severity, a sensitive
  path (constrained, and annotated), a missing `base_schema`, and a chain that
  never reaches a type; valid policies still load beside broken ones;
- digest stability under the same source, under comment and description
  edits, and its sensitivity to constraints, to a base policy and to
  severity.

`fieldpath.HasPrefix` has its own table test.

`CGO_ENABLED=0 go test ./...` passes. `CGO_ENABLED=0 mise exec -- make lint`:
0 issues.

## Limitations

- Pattern constraints (`[string]: T`) are checked by CUE for conflicts. They
  are not walked for absences or rule attribution: a violation under a
  pattern takes the rule of the nearest named ancestor, or its own path.
- Constraints that refer to another field the record lacks (`size: <=max`
  without `max`) are incomplete, so they are neither a conflict nor an
  absence.
- Conflicts between chain members themselves, which involve no data, are not
  reported at load.
- Attribute validation is strict across the whole chain, including attributes
  inherited from the type. A leftover non-policy `@pudl(...)` key on a type
  field, such as the old `binding=`, makes that type's policies fail to load.
- The tree walk stops at depth 32 for recursive definitions.

## Line delta (`git diff --numstat`)

- Production: +942 / -0 (`internal/policy` 936, `internal/fieldpath` 6).
- Tests: +511.
