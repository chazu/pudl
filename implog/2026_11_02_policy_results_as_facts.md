# Policy results as facts, `pudl policy evaluate` — pudl-xf5.8.3 (+ pudl-udi observe wiring)

Delivery step 3 of `docs/design/2026-11-01-compliance-policies.md`. The engine
(`internal/policy`, pudl-xf5.8.2) is now wired into ingest; its results are
bitemporal facts. The remaining part of pudl-udi (likely-secret warnings at
observe ingest) is included because it touches the same ingest code.

## Storage

- Relations (both reserved, see below):
  - `policy_status(resource, policy, type, status, policy_digest)`
  - `policy_violation(resource, policy, rule, severity, path, kind, declared_by)`
- Status is `compliant`, `noncompliant` or `unknown`. An absence that is not a
  violation makes the status `unknown` and is **not** a violation fact. Facts
  never carry record values. Provenance is `{"entry_id", "type"}`.
- Source: `database.PolicySource(policy, resource)` = `policy:<policy>:<resource>`.
  Reconciled with the projection reconcile: `CatalogTx.ReconcileProjection`
  now accepts policy sources as well as projection sources. Changed results
  are closed as observations (`valid_end`), so history keeps every previous
  result and the digest it was evaluated with; orphaned results (entry deleted
  or re-identified) are retracted as corrections, as projection does.
- `policy_state(resource_id, policy, entry_id, type, digest, status,
  evaluated_at)`, migration 26: which entry and digest each result describes.
  It gives `pudl policy list` (pudl-xf5.8.4) its "last evaluated" and counts.
- Reserved names: `database.IsReservedRelation` covers both relations
  (`ReservedRelations` lists them for completion). `addFactIn` accepts them
  only under a policy source; general writers (`AddFact`, `Tx.AddFact`,
  `CatalogTx.AddFact`, `pudl facts add`) cannot use the relations or the
  `policy:` source prefix. Rule lint and check diagnostics already skip reserved
  relations, so a check over `policy_noncompliant` needs no producing schema.

## Evaluation points

- **Import**: documents, collection items, re-imports (`reobserve`,
  `reobserveDocument`) evaluate in the transaction that projects facts, through
  one helper, `importer.observeFacts`. A re-import of an unchanged document is
  re-observed when policies apply to it (as when its schema projects).
- **Observe** (`#CommandObserve`): each spooled descriptor carries its
  `*policyfacts.Prepared`; `Commit` makes it current for new and deduplicated
  records. `ObserveIngestResult.PolicyChange` counts updates and closures.
- **Complete scope exit**: when a complete observation of scope S commits,
  `policyfacts.CloseScopeLeavers` invalidates the results of resources whose
  results came from an earlier complete snapshot of S and that this snapshot
  does not contain (`CatalogTx.ScopeLeavers`, one query). A partial observation
  closes nothing.
  - Finding: latest-known *projected* facts are not closed on scope exit;
    projection establishes absence only inside evidence views
    (`docs/observation-scope.md`). Policy results therefore do both: they are
    closed in latest-known on a complete scope exit (the ticket's wording), and
    `evidence.Open` evaluates policies over the selected records only, so a
    check selecting `scope:S` or a snapshot sees results for that population
    and nothing else. Without the second half, a check over
    `policy_noncompliant` with evidence selectors would have passed vacuously.
- Records without a resolved identity are skipped, as projection skips them:
  with content-hash identity every old result would stay current.
- A policy that fails to load keeps its existing results (`Prepared.Kept`), as a
  broken projection spec does. A record a policy fails to evaluate keeps its
  result and is reported.

## Public API

```go
// internal/policyfacts
type LoadFunc func(entry database.CatalogEntry) (any, error)
func New(src policy.Source) *Evaluator
func (e *Evaluator) Set() *policy.Set
func (e *Evaluator) LoadErrors() []policy.LoadError
func (e *Evaluator) Applicable(typeName string) []*policy.Policy
func (e *Evaluator) Prepare(typeName, entryID, resourceID string, resolved bool, record any) *Prepared
func (e *Evaluator) Select(names []string) ([]*policy.Policy, error)
func (e *Evaluator) Refresh(ctx, db, load LoadFunc, opts Options) (Report, error)
func (e *Evaluator) Preview(ctx, db, load LoadFunc, policies []*policy.Policy) (Report, error)
func (e *Evaluator) Stale(db *database.CatalogDB) ([]string, error)
type Prepared struct{ ResourceID, EntryID, Type string; Results []*policy.Result; Scope, Kept, Errors []string }
type Options struct{ Policies []string; DryRun bool }
type Change struct{ Updated, Closed int }
type Report struct{ Resources int; Policies []PolicySummary; TopViolations []RuleSummary; Change; Orphaned int; Errors, LoadErrors []string }
type PolicySummary struct{ Policy, Type, Digest string; Compliant, Noncompliant, Unknown, Provisional int }
type RuleSummary struct{ Policy, Rule string; Severity policy.Severity; Kind policy.Kind; Resources int }
func Facts(resourceID, entryID string, r *policy.Result) []database.Fact
func Observe(tx *database.CatalogTx, p *Prepared, mode database.ProjectionMode) (Change, error)
func CloseScopeLeavers(tx *database.CatalogTx, scope, snapshotID string) (int, error)
type Draft struct{ Evaluator *Evaluator; Policies []*policy.Policy }
func LoadDraft(file string, schemaPaths []string) (*Draft, error)
func (d *Draft) Select(names []string) ([]*policy.Policy, error)
func (d *Draft) Close()

// internal/database
const PolicySourcePrefix = "policy:"; PolicyStatusRelation, PolicyViolationRelation
func PolicySource(policy, resourceID string) string
func IsPolicySource(source string) bool
type PolicyState struct{ ResourceID, Policy, EntryID, Type, Digest, Status string; EvaluatedAt int64 }
func (t *CatalogTx) PolicyStates(resourceID) / PutPolicyState / DeletePolicyState / ListPolicyState / PolicyOrphans / ScopeLeavers(scope, snapshotID)
func (c *CatalogDB) ListPolicyState() ([]PolicyState, error)
func (t *CatalogTx) / (c *CatalogDB) CurrentResourceEntries() ([]CatalogEntry, error)

// internal/policy:      func (s Severity) Rank() int
// internal/projection:  func (r *Registry) Source() redact.Source
// internal/secretscan:  type Warning; type Tally{Warnings; Total}; (*Tally).Note; const MaxWarnings
//                       (importer.SecretWarning is now an alias of secretscan.Warning)
// internal/observe:     ObserveIngestResult.SecretWarnings, SecretWarningsTotal, PolicyChange
```

`CurrentResourceEntries` picks, per resolved resource, the entry its policy
results (else its projected facts) were last computed from, falling back to
the most recently imported entry, so an observation that deduplicated to an
older entry is not mistaken for the newest import.

## Commands

- `pudl policy` — parent; `cmd/policy.go` holds `loadPolicies()`, which
  `list` and `show` (pudl-xf5.8.4) reuse as new files beside `evaluate`.
- `pudl policy evaluate [POLICY...] [--dry-run] [--what-if FILE]`:
  - Re-evaluates every stored current resource from its retained payload in
    one transaction; closes results of policies no longer applicable or no
    longer loaded, and orphaned results. Named policies limit both.
  - `--dry-run` runs the same transaction and rolls it back, so its counts are
    exact.
  - `--what-if FILE` copies the draft into a temporary schema root searched
    first (namespace: its directory relative to the schema path holding it,
    else its CUE package name), so it shadows an installed policy of the same
    name; it evaluates only the draft's policies and writes nothing. Errors:
    the draft does not load, declares no policy, or one of its policies fails
    to load. `--what-if` with `--dry-run` is rejected.
  - Human output: per-policy compliant/noncompliant/unknown counts (with
    provisional counts), the top 10 violated rules by severity then resources,
    and `Results: N updated, N closed, N orphaned closed`. JSON:
    `{"report_version": 1, "mode": "evaluate|dry-run|what-if", "draft", ...Report}`.
- `pudl doctor`: new repair `policy-results` (after `projections`). Diagnose
  reports stale results (digest differs from the loaded policy's), results of
  unloaded policies, results whose entry was deleted, re-identified, reassigned
  or superseded, and never-evaluated pairs; load errors are listed as errors.
  `--fix` runs the evaluation. It follows `policy-leaves`, so moved entries get
  their compliance back.

## Built-in rule

`internal/importer/bootstrap/pudl/rules/policy.cue` (installed with the other
bootstrap rules, including into existing workspaces on the next import):

```cue
policy_noncompliant: {
	head: {rel: "policy_noncompliant", args: {resource: "$R", policy: "$P", severity: "$S"}}
	body: [
		{rel: "policy_status", args: {resource: "$R", policy: "$P", status: "noncompliant"}},
		{rel: "policy_violation", args: {resource: "$R", policy: "$P", severity: "$S"}},
	]
}
```

One tuple per severity present among a result's violations; constrain
`severity` for a threshold. The `#Check` schema is unchanged.

## pudl-udi: observe-ingest secret warnings

`observeRoute` builds a `secretscan.Scanner` beside the redactor and scans
each record after redaction with the intended and assigned schemas
(`prepareObserveRecord`). Warnings (record index, schema, path, kind; capped
at 50, total counted) are in `ObserveIngestResult`, printed on the run's
stderr with the import's `secretNotes` wording, and in the populate report
JSON (`populate.secret_warnings`, `secret_warnings_total`). Stored data and
hashes are unchanged. The importer's tally moved to `secretscan.Tally` so both
ingest paths share it. `cmd/run*` edits are 9 lines.

## Tests

- `cmd/policy_evaluate_test.go`: import writes results (2 noncompliant: a
  conflict and an absence opted into `absent="violation"`; 1 unknown); `pudl
  query` over `policy_status`, `policy_violation` (absent kind, declared_by)
  and `policy_noncompliant severity=high`; doctor `policy-results: ok`;
  `--what-if` on a draft with JSON output and an unchanged fact store
  (count and max `tx_seq`); a policy edit makes doctor report "4 policy results
  are stale"; `--dry-run` writes nothing; `evaluate` updates, doctor is ok
  again, current facts carry the new digest and history keeps both; `pudl
  facts add --relation policy_status` is refused.
- `internal/policyfacts`: observe writes results; Datalog over both relations
  and the shipped rule; `CheckDiagnostics` for a check over
  `policy_noncompliant` is empty; a partial observation closes nothing; a
  complete one without a resource closes its results (history kept); evidence
  views hold results for the selected snapshot only; `Preview` writes nothing;
  deleting an entry makes `Stale` report and `Refresh` close the orphan;
  unknown absences are not violation facts; reserved relations and the
  `policy:` source are refused by `AddFact`.
- `internal/observe`: an observed catchall record with a GitHub token warns
  with record 1, path `auth.token`, kind `github_token`, no value in the
  result's JSON, stored bytes unchanged.
- `cli_json_contract_test`: `policy evaluate` and `--dry-run` emit one JSON
  document.

`CGO_ENABLED=0 go test ./...` passes; `CGO_ENABLED=0 mise exec -- make lint`:
0 issues; `make check-skills` passes; `make docs` and the help golden are
regenerated.

## Line delta (`git diff --numstat sre-focus`, after merging sre-focus)

- Production Go: +1540 / −92 (net +1448). New: `internal/policyfacts` 789,
  `internal/database/policy_state.go` 200, `cmd/policy*.go` 198,
  `internal/repair/policy_results.go` 61, `internal/secretscan/tally.go` 33.
  Wiring: database 55/−12, evidence 58/−28, importer 38/−33, observe 75/−10,
  cmd (doctor, warnings, run populate/report) 18/−6, policy 3, projection 8,
  repair 4/−3.
- Tests: +414 Go test lines.
- CUE: +19 (`policy.cue`). Generated: `help.golden.json` +58/−2,
  `cli-reference.md` +72/−1.

## Limitations

- A resource whose type had no applicable policy when it was last ingested,
  and whose schema does not project, is re-evaluated from its most recently
  imported entry; if its last observation deduplicated to an older entry, that
  older state is not detected.
- The evaluation holds the catalog write transaction while it reads payloads,
  like the projection sync.
- A what-if draft must be standalone CUE; imports from other packages do not
  resolve in the temporary root.
- `pudl import --dry-run` does not preview policy results.
