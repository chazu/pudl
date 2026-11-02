# `pudl policy list`, `pudl policy show`, compliance in `pudl show` — pudl-xf5.8.4

Delivery step 4 of `docs/design/2026-11-01-compliance-policies.md` (with the
`pudl show` compliance section folded in from `.8.5`). `pudl list --policy`
was dropped: `pudl policy show` lists the failing resources.

Everything reads stored results — `policy_state` rows and the current
`policy_violation` facts — through `internal/policyfacts`; nothing is
re-evaluated. A result is stale when its digest differs from the loaded
policy's (the same test `Stale`, the doctor repair, uses; it now calls
`IsStale`).

## Commands

- `pudl policy list [--type T] [--json]`: one row per loaded policy: name,
  type, extends, rule count (inherited included), the type's review status,
  last evaluated (`never` before any evaluation), compliant / noncompliant /
  unknown / stale counts. `--type T` lists the policies that apply to T (its
  ancestors' policies included, via `Applicable`). Load errors go to stderr
  (JSON: `load_errors`). Before any evaluation the counts are zero.
  JSON: `{"report_version": 1, "policies": [Summary...], "load_errors"?}`
  where a Summary is `{policy, type, policy_digest, compliant, noncompliant,
  unknown, provisional, extends?, rules, review, stale, last_evaluated?}`.
- `pudl policy show POLICY [--limit N] [--json]`: type and review, extends,
  chain, last evaluated, result counts with a stale warning, every rule with
  severity, doc, declaring policy and the number of resources failing it (a
  violation of an unannotated constraint gets its own row, named by its
  path), and the failing resources ordered by highest severity then violation
  count, capped by `--limit` (default 10, 0 = all). Unknown policy names fail
  with the loaded names (or the policy's load error).
  JSON: `{"report_version": 1, "summary": Summary, "chain": [...], "rules":
  [{name, severity, doc?, paths, declared_by_policy, failing}], "failing":
  [Stored...], "failing_total": N}`.
- `pudl show ID [--json]`: a `Compliance:` section after the entry fields,
  one line per policy applicable to the entry's type: status (`compliant`,
  `noncompliant`, `unknown`, or `not_evaluated` when no result is stored),
  `(provisional: type is a draft)`, the derived highest severity, a stale
  marker, then each violation (severity, rule, path, kind, declaring policy).
  JSON adds `"compliance": [{...Stored, provisional, stale}]` (always an
  array). A failure to load schemas or results is a stderr warning; the rest
  of `show` still prints.

## Public API

```go
// internal/policyfacts (stored.go)
type Stored struct {
	database.PolicyState
	HighestSeverity policy.Severity    // derived when read
	Violations      []policy.Violation // current policy_violation facts, sorted
}
func ReadStored(db *database.CatalogDB, keep func(database.PolicyState) bool) ([]Stored, error)
func (e *Evaluator) Review(typeName string) validator.SchemaReview
func (e *Evaluator) IsStale(s database.PolicyState) bool
type Summary struct {
	PolicySummary
	Extends string; Rules int; Review string; Stale int; LastEvaluated int64
}
func (e *Evaluator) Summarize(states []database.PolicyState, policies []*policy.Policy) []Summary

// internal/policy
func (s *Set) Review(typeName string) validator.SchemaReview
func SortViolations(vs []Violation) // was sortViolations
```

`cmd`: `policy_list.go` (with `openPolicies`, shared by show),
`policy_show.go`, `show_compliance.go` (`entryCompliance`, `printCompliance`);
`show.go`/`show_json.go` gain one call each.

## Tests

`cmd/policy_list_show_test.go`, on a fixture with a validated `#Bucket` type
carrying `#BucketBP` and `#BucketPCI` (extends BP) and a draft `#Queue` type
carrying `#QueueBP`:

- list before any import: three policies, PCI extends BP with 2 rules,
  validated/draft review, `never`, zero counts (JSON and the human table);
- after import: PCI 1/2/0, BP 2/1/0, QueueBP 1 noncompliant and provisional;
  `--type pudl/aws.#Queue` lists only QueueBP;
- show PCI (JSON): chain, rule rows with declaring policy, doc and failing
  counts, the resource failing both rules first at `critical`; human output
  with `--limit 1` (`Failing resources (1 of 2)`); an unknown policy errors;
- `pudl show` of that entry (JSON and human): two policies, violations with
  path, kind and the inherited rule's declaring policy; the queue entry is
  `noncompliant (provisional: type is a draft)`;
- after a policy edit: PCI has 3 stale results, BP none, `show` marks the PCI
  result stale, and a newly added policy is `not_evaluated`.

`cli_json_contract_test`: `policy list` before any policy, then with an
installed policy `policy list --type` and `policy show` emit one JSON
document; `show --json` keeps doing so with the compliance array.

`CGO_ENABLED=0 go test ./...` passes; `CGO_ENABLED=0 mise exec -- make lint`:
0 issues; `make check-docs` and `make check-skills` pass. `make docs`, the
help golden and the `pudl-core` skill (one bullet for `policy list/show` and
the show section) are regenerated.

## Line delta (`git diff --numstat sre-focus`, after merging sre-focus)

- Production Go: +501 / −7 (net +494). New: `cmd/policy_show.go` 165,
  `internal/policyfacts/stored.go` 124, `cmd/policy_list.go` 101,
  `cmd/show_compliance.go` 87. Wiring: `cmd/show.go` +6 (help text and one
  call), `cmd/show_json.go` +3/−1, `internal/policy` +13/−4 (Review,
  SortViolations export), `policyfacts/stale.go` +2/−2. About a fifth of the
  new lines are command help text and doc comments; the target of ~400 was
  exceeded mainly by the show section's warning-not-failure handling and the
  per-rule rows for unannotated constraints.
- Tests: +209 / −1 Go test lines.
- Generated: `help.golden.json` +56/−1, `cli-reference.md` +70. Skill +2.

## Limitations

- `pudl show` now loads the schema set (to find applicable policies and the
  type's review), which costs a schema load per invocation.
- `ReadStored` reads every current `policy_violation` fact and filters in Go;
  fine at current scales, an obvious place for a relation-and-arg filter if
  the simplification pass (`pudl-4fx.6`) wants one.
- Results are per resource: `pudl show` of an older entry of a resource shows
  the resource's current results (their `entry_id` says which entry they
  describe).
- Unknown absences are not stored as facts, so `policy show` counts failing
  resources per rule but not unknowns per rule.
