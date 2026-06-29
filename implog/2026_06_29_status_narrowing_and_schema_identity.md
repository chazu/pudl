# 2026-06-29 — Convergence frontier: status narrowing + schema-driven inventory identity

Two self-contained items off the convergence frontier
(`docs/system-models-build-status.md`).

## 1. Status: collapse `converged` into `clean`

Started by distinguishing `converged` (verified ∅ after convergence) from `clean`
(observe baseline), but on review they name the **same state** — observed == desired
— differing only in *provenance*, which already lives in the run history/report, not
the status enum. So they were **collapsed into a single in-sync status, `clean`**, and
`converged` was removed from the vocabulary. New status set:
`unknown | drifted | converging | clean | failed`.

- `runVerdict` (`cmd/run.go`): a clean drift (`r.Drift.Clean`) → `clean`; a converge
  run whose outcome reached ∅ → `clean`. (No `convergent` param; the earlier draft of
  this had one — reverted.)
- The converge loop's success outcome constant was renamed `outcomeConverged "converged"`
  → `outcomeClean "clean"` (`cmd/run_converge.go`); `ConvergeReport.Outcome` and the
  run report/plan text use `clean`.
- `converged` removed from `UpdateStatus`'s valid set (`catalog_status.go`), the struct
  comment (`catalog.go`), `pudl status` help + color map (`cmd/status.go`), and the
  display strings.
- The provenance narrowing still holds: `ingest-manifest` writes `converging` on a bare
  apply (exit 0); only the drift re-check writes the verified `clean`.
- Docs updated: pudl `architecture.md`, `cli-reference.md`, `system-models-build-status.md`,
  and the **canonical mu spec** `V1-BUILD-SPEC.md` §5/§8 (records the 2026-06-29
  collapse decision).

### Per-resource `converging → clean` promotion (the drift re-check closes the loop)

`ingest-manifest` writes `converging` per *resource* (`targetToDefinition(action.Target)`,
e.g. `web`) — a separate axis from the model-instance verdict (`//models/<name>`).
Nothing previously promoted those rows once the apply was verified. Now, when a
`pudl run` drift re-check is **clean**, `promoteConvergingResources` (`cmd/run.go`)
flips this model's resources from `converging` to `clean` via the new
`CatalogDB.PromoteConvergingToClean(defs)` (`internal/database/catalog_status.go`).

Scoping: the manifest action targets are bare resource names with no model linkage,
so the promotion is scoped to the **definition names derivable from the model's own
desired records** (`modelResourceDefs` — each record's identity_fields, else
name/path/id). It only flips rows currently in `converging`, so it can never touch
another model's pending resources, and a resource whose defName can't be derived is
simply left as-is (never wrongly promoted). Best-effort: a missing catalog/resolver
never fails the run. Unit test: `TestPromoteConvergingToClean` (in-scope converging →
clean; non-converging untouched; out-of-scope model untouched).

## 2. Schema-driven identity for inventory drift

Inventory drift (`pudl run --from-catalog`) set-diffs desired vs observed records by
a per-record identity key. `recordIdentity` (`cmd/run_inventory.go`) used a hardcoded
heuristic: `_schema` + the first present of `name | path | id`.

It now keys records by the schema's **declared `identity_fields`**, resolved from the
inference graph:

- new `identityResolver func(schema string) []string` + `schemaIdentityResolver()`
  (backed by `inference.SchemaInferrer.GetSchemaMetadata(...).IdentityFields`);
- `recordIdentity` builds a composite key from those fields, and **falls back** to the
  old `name|path|id` heuristic when a schema declares no `identity_fields` or a declared
  field is absent from the record (so existing linux/fs/k8s shapes are unaffected);
- threaded through `inventorySetDiff` and `runInventoryDrift`; `cmd/run.go`'s
  `--from-catalog` branch builds the real resolver.

This makes inventory matching consistent with how the catalog computes resource
identity everywhere else (`identity.ComputeResourceID`), and fixes mis-matching for
resources whose identity is composite or not `name/path/id` (which previously produced
false `missing`/`changed` or were skipped as un-keyable).

## API / behavior delta

- Status vocabulary: `converged` removed; `clean` is the single in-sync status.
- `recordIdentity(rec, identityResolver)`, `inventorySetDiff(desired, observed,
  identityResolver)`, `runInventoryDrift(db, origin, desired, identityResolver)` —
  all gained the resolver parameter (pass `nil` for fallback-only behavior).

## Verification

`CGO_ENABLED=0 go build ./...` clean; `CGO_ENABLED=0 go test ./...` all green.
Updated tests: `TestRunVerdict` (drift/converge → `clean`), `catalog_status_test`
(valid set drops `converged`), `status_test` (color cases); new
`TestInventorySetDiff_SchemaDrivenIdentity` (composite identity matching, no
name|path|id present). Frontier + canonical mu spec updated.
