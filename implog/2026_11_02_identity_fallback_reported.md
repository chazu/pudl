# Inventory identity fallback is one function and is reported — pudl-167

Inventory drift and snapshot comparison key records by the schema's declared
`identity_fields`. When the schema is unknown or declares none, identity fell
back silently to the first present of `name|path|id`. A schema whose records
share a name, or are identified by another key, could merge distinct resources
or report false missing drift, and nothing told the user.

## Where the fallback lived

The ticket listed three copies. On `sre-focus` two remained:

- `internal/acute/inventory_diff.go` (`RecordIdentity`);
- `internal/acute/drift_previous.go` (`fallbackIdentityFields`, used to compare
  the identity a desired record used with the identity retained on an earlier
  snapshot's records).

The `cmd/run_inventory.go` desired-definition copy no longer existed: it was
removed with the convergence workflow (`76a72ec`).

## Public API

```go
// internal/acute (inventory_diff.go)
// IdentityFields returns the declared identity_fields, or the first present
// of name|path|id with fallback=true. The only place the fallback is decided.
func IdentityFields(rec map[string]any, identity IdentityResolver) (fields []string, fallback bool)
// IdentityFallbacks maps each schema identified by the fallback to the keys
// used (first-seen order), over desired and observed records.
func IdentityFallbacks(identity IdentityResolver, desired []map[string]any, observed []ObservedRecord) map[string][]string
// FallbackNotes renders the map as sorted human lines.
func FallbackNotes(fallbacks map[string][]string) []string

// internal/acute (drift.go)
type ModelDriftResult struct {
	// ...
	IdentityFallbacks map[string][]string `json:"identity_fallbacks,omitempty"`
}
```

Removed: `acute.fallbackIdentityFields`, and `acute.sameFields` (replaced by
`slices.Equal` in the same comparison).

## Behavior

- `RecordIdentity` and the previous-snapshot comparison (`attachPrevious`)
  both call `IdentityFields`, so they cannot disagree about which key a record
  used. Keys and labels are unchanged (fallback keys still format with `%v`).
- **Inventory drift** (`pudl run` on a model with `desired`) sets
  `identity_fallbacks` on the drift result: one entry per schema, listing the
  fallback keys its desired and observed records used. The JSON report carries
  it under `drift.identity_fallbacks`. The human report adds one line per
  schema to the drift section:
  `- identity fallback: pudl/linux.#Package declares no identity_fields (or is not loaded); records matched by name`.
- **Snapshot comparison** (`pudl snapshot show <after> --compare <before>`)
  reports the same over both snapshots' records: `identity_fallbacks` in
  `--json` (always present, `{}` when none), and an `identity fallback:` line
  per schema after the summary line in human output.
- The check does not fail closed. Verdicts and exit codes are unchanged.

## Is a stricter mode warranted?

Not as the default. Today's desired shapes for resource-type tags (`linux`,
`fs`, `k8s`, `git.repository`) often route to schemas that declare no
`identity_fields`, and many fixtures and examples rely on the fallback, so
failing closed would turn working models into failures with no new
information. An opt-in strict mode is warranted later, once the content packs
declare `identity_fields` on every inventory-observed schema: a flag on the
`pudl check` replacement for `pudl run` (being built separately) that turns a
non-empty `identity_fallbacks` into an uncertain verdict, in line with the
2026-10-10 evidence work (unobserved means uncertain, not absent). A cheaper
first step is a `pudl doctor` check that flags inventory-observed schemas with
no `identity_fields`.

## Tests

- `internal/acute/identity_fallback_test.go`: matching and previous-value
  lookup share `IdentityFields` (a `path`-keyed record is matched and its
  previous value found); `IdentityFallbacks` reports a schema without
  `identity_fields` once with every key used, skips declared schemas and
  records with no identity; the human drift report renders the line. The
  existing previous-value fallback test moved here from
  `drift_previous_fallback_test.go`.
- `cmd/run_inventory_test.go`: drift over a real catalog reports
  `{"pudl/linux.#Package": ["name"]}` once for five records.

## Docs

`docs/system-models-build-status.md` no longer says schema-driven identity is
a follow-up; it describes declared identity, the fallback and its reporting.

## Line delta against sre-focus

- Production: +88 / −64 (net +24). Consolidation removed 31 net lines in
  `drift_previous.go`; the diagnostic (`IdentityFallbacks`, `FallbackNotes`,
  the drift field and the two renderers) added the rest.
- Tests: +60 / −16 (net +44).

## Verification

- `CGO_ENABLED=0 go test ./...`: all packages pass.
- `CGO_ENABLED=0 mise exec -- make lint`: 0 issues.
