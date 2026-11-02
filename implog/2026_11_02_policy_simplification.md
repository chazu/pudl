# Simplification pass over compliance policy code — pudl-4fx.6

A review of the policy engine (`internal/policy`), result persistence
(`internal/policyfacts`, `internal/database/policy_state.go`) and the policy
commands (`cmd/policy*.go`, `cmd/show_compliance.go`) for duplication and
excess, with no behavior change. Existing tests, the JSON contract and the help
golden are unchanged.

## What changed

- **Stored results are filtered in SQL.** `ReadStored(db, resourceID,
  policyName)` replaces `ReadStored(db, keep func)`; `""` matches any.
  `CatalogDB.ListPolicyState(resourceID, policy)` filters rows in SQL, and the
  new `CatalogDB.PolicyViolations(resourceID, policy)` joins the matching
  `policy_state` rows to `current_facts` on their policy source
  (`policy:<policy>:<resource>`, indexed by `idx_current_facts_source`). Before,
  every current `policy_violation` fact was read and matched in Go. `pudl show`
  reads one resource's results; `pudl policy show` reads one policy's.
- **`CurrentResourceEntries` picks each resource's entry in SQL** (the entry
  its results or projection were last computed from, else the newest import),
  instead of a Go pass over every entry with an extra scanned column. The
  `scanWithExtra` helper and the unused `CatalogTx.ListPolicyState` are gone.
  The new query was checked against the old one on 40 randomized catalogs (280
  resources, with timestamp and `evaluated_at` ties) with identical results.
- **policyfacts reuses projection's payload loader type**
  (`projection.LoadFunc`) instead of declaring an identical `LoadFunc`.
- **Status counting is one method**, `PolicySummary.count`, shared by the
  evaluation report (`tally.note`) and the stored-result summary
  (`Summarize`).
- **`Refresh` and `Preview` share one loop**, `evaluateStored`, over the stored
  current entries (payload read only when a selected policy applies, results
  tallied, then observed by `Refresh` and discarded by `Preview`).
- **Smaller helpers.** `Prepared.covers`, `intersect` and the draft's policy
  check use `slices`; `Evaluator.Select` builds its "loaded" list inline;
  `Draft.Select(nil)` returns every draft policy, so `whatIfPolicies` no longer
  special-cases it; the draft stages its file and module file in one loop.
- **Engine:** `recordPath` follows CUE error labels through the record in one
  pass, replacing `parseErrorPath` (to an intermediate `errorSegment` type)
  plus `resolveInRecord`. Same rules: quoted keys unquoted, hidden and
  definition labels rejected, non-negative digits as list indices.
  `containsString` is `slices.Contains`.

## Findings

### Duplication with `internal/projection`

Little was left to share. Reconciliation is already one function
(`CatalogTx.ReconcileProjection` accepts projection and policy sources), and
the evidence view already evaluates policies in the same transaction and loop
that materializes projected facts. What remained was the duplicate `LoadFunc`
type (now shared) and two near-identical stored-entry loops inside
policyfacts (now one). Projection does not close results on a scope exit, so
`ScopeLeavers`/`CloseScopeLeavers` have no projection twin. `closeResult`
(policy) and `retire` (projection) both empty a source and drop a state row,
but in different state tables; a shared helper would save nothing.

### `policy_state` is kept

Its contents cannot be derived from the current `policy_status` facts without
changing behavior:

- **`entry_id` goes stale in fact provenance.** A reconcile compares facts by
  relation and arguments only. When a resource is re-observed through a new
  entry and its result is unchanged, no fact is written, so the provenance
  `entry_id` still names the first entry that produced that result.
  `policy_state.entry_id` is updated on every evaluation. `PolicyOrphans`
  (retracts results whose entry was deleted or re-identified), `ScopeLeavers`
  (snapshot membership of the entry), `CurrentResourceEntries` (dedup to an
  older entry), doctor's "superseded" reason and the `entry_id` shown by `pudl
  policy show` and `pudl show` all need the current entry. With provenance, a
  pruned old entry would retract a valid result, and a deleted new entry would
  leave an orphan undetected.
- **`evaluated_at` is the last evaluation, not the last change.** It is
  rewritten on every evaluation; a status fact's `tx_start` only moves when
  the result changes. `pudl policy list`'s "last evaluated" would become "last
  changed".
- Putting `entry_id` into the facts' arguments would write a new fact on every
  re-observation and change fact history. Keeping a slimmer table (entry and
  time only) and joining facts for status, type and digest would add a
  table-altering migration and a join to every read: more code, not less.

So there is no migration in this pass.

### `pudl show` schema loading

Not made lazy. The compliance section lists every policy applicable to the
entry's type, including `not_evaluated` ones with no stored result, and
applicability comes from the inheritance graph of the full schema set. There
is no cheaper test for "a policy could apply" that keeps that output. `--field`
and `--history` already skip the section.

### `--what-if` draft loader

It already uses the normal schema loading (`inference.NewSchemaInferrer`) with
the draft's temporary root searched first. What remains is staging that root.
The `cue.mod/module.cue` file stays: the loader works without it in the test,
but CUE would then search parent directories of the temporary directory for a
module root.

### Rendering

The human output of `policy evaluate`, `policy list`, `policy show` and the
`pudl show` section have different shapes (per-policy counts, a table, rule
rows, per-violation lines); the shared parts (`evaluatedAt`, `orDash`, status
counting) were already shared or are now. Text and JSON are unchanged.

### Engine

Conservative: only the error-path walk and one helper changed. The rest
(tree merge, attribute validation, absence walk, digest) is covered closely by
its tests and has no redundant layer.

## Public API changes (internal packages)

```go
// internal/database
func (c *CatalogDB) ListPolicyState(resourceID, policy string) ([]PolicyState, error) // was ListPolicyState()
func (c *CatalogDB) PolicyViolations(resourceID, policy string) ([]string, error)      // new: args JSON
// removed: func (t *CatalogTx) ListPolicyState() ([]PolicyState, error)

// internal/policyfacts
func ReadStored(db *database.CatalogDB, resourceID, policyName string) ([]Stored, error) // was keep func
func (e *Evaluator) Refresh(ctx, db, load projection.LoadFunc, opts Options) (Report, error)
func (e *Evaluator) Preview(ctx, db, load projection.LoadFunc, policies []*policy.Policy) (Report, error)
func (d *Draft) Select(names []string) ([]*policy.Policy, error) // no names: every draft policy
// removed: type LoadFunc (use projection.LoadFunc)
```

## Tests

No test changed. `CGO_ENABLED=0 go test ./...` passes (38 packages ok).
`CGO_ENABLED=0 mise exec -- make lint`: 0 issues.
`mise exec -- make check-docs check-skills` passes.

## Line delta (`git diff --numstat sre-focus`)

- Production Go: +182 / -267, net -85: `internal/policyfacts` -48,
  `internal/policy` -33, `internal/database/policy_state.go` -2 (after adding
  `PolicyViolations`), `cmd` -2. That is about 3% of the ~2,900-line feature;
  the persistence layer turned out to share its core with projection already,
  and `policy_state`, the largest candidate, has to stay.
- Tests: 0.
