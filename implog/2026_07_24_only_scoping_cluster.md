# `--only` scoping cluster: model row, checks, promotion fallback

Closes the three P2 defects that shared one root cause — `--only` scope resolved
correctly at the plan, then leaked at the edges where a run writes what it
concluded. All three are governing invariant 2 ("the effective scoped model is
used consistently for planning, execution, report scope, resource promotion, and
any scope-sensitive checks").

| Issue | Defect | Report # |
|-------|--------|----------|
| `pudl-x8n` | Model-level status written unscoped under `--only` | 8 |
| `pudl-s7t` | `PromoteConvergingToClean` fallback not model-scoped | 9 |
| `pudl-m1m` | Checks receive the original, not the effective model | 10 |

## pudl-x8n — a scoped `clean` no longer generalizes

`persistRunStatus(model.Name, verdict, live)` wrote the run's verdict onto the
model instance row unconditionally. Under `--only` the run observed a subset, so
a ∅ over the named resources became a whole-model `clean` — read downstream by
`pudl status`, `pudl model list`, and `checkUpstreamFreshness`, the one feature
whose job is spotting stale upstreams.

Only `clean` fails to generalize from a subset to the whole. `drifted` and
`failed` remain true statements about the model (a defect in a subset *is* a
defect in the model), and `unknown` is already the weakest claim. So the fix is
a single degradation, not a blanket skip:

```go
func modelRowVerdict(verdict string, restricted bool) string {
	if restricted && verdict == "clean" {
		return "unknown"
	}
	return verdict
}
```

The run row still records the run's *real* verdict — the model row is the
generalization, the run row is the observation. To keep the resulting `unknown`
distinguishable from the `unknown` of a lost manifest receipt, the run row also
gets a note (`runFinishState.note`, new) naming the scope:

    verdict "clean" covers only the --only scope (web,api); model status left "unknown"

`finishRunRecord` now joins that note with the run error rather than letting
either overwrite the other. A live run prints the same note, with a pointer to
re-run unscoped.

## pudl-s7t — fallback promotion excludes other models' rows

`PromoteConvergingToClean` promoted `converging → clean` by bare `target` name
with no model predicate, while `cmd/run.go` claimed it "cannot touch another
model's pending resources". Two models each declaring a resource named `nginx`
produce the same target key, so model A's clean drift promoted model B's pending
row — reachable via the untagged `ingest-manifest` path.

The SQL now also requires the row's model tag to be absent or this model's:

```sql
AND (json_extract(tags, '$.model') IS NULL OR json_extract(tags, '$.model') = ?)
```

That covers everything the row data can support: rows tagged to another model are
excluded, this model's own tagged rows and genuinely untagged rows still promote
(the case the fallback exists for). Untagged rows from two models sharing a
target name remain indistinguishable — the row records no applying model at all —
and both comments now say that instead of over-claiming. `ingest-manifest
--model` routes around the fallback entirely.

## pudl-m1m — checks consume the effective model

`runChecks(model, modelDir)` → `runChecks(effectiveModel, modelDir)`, guard
included. Behaviour-identical today (`--only` requires `--converge`, which takes
the other branch), but it is the shape invariant 2 requires and it precedes the
D3 work on scoping checks by datalog constraint.

## Public API

```go
// internal/database — signature change (one caller: cmd.promoteConvergingResources)
func (c *CatalogDB) PromoteConvergingToClean(targets []string, model string) (int, error)

// cmd — new, unexported
func modelRowVerdict(verdict string, restricted bool) string

// cmd — runFinishState gains a note field carried into the run row
type runFinishState struct {
	verdict           string
	outcome           string
	needsVerification bool
	note              string
}
```

## Tests

Unit:

- `TestModelRowVerdict` — 10 cases over the {verdict × restricted} matrix.
- `TestScopeModelForRun_PreservesChecks` — guards the risk the m1m fix
  introduces: the run now guards on `len(effectiveModel.Checks)`, so scoping
  dropping checks would silently skip them rather than run the wrong ones.
- `TestPromoteConvergingToClean` updated for the new signature.

Against a real catalog (temp `HOME`, no mu required):

- `TestScopedCleanRun_ModelRowUnknown_RunRowKeepsVerdict` — drives
  `startRunRecord` → `persistRunStatus` → `finishRunRecord` and asserts the model
  row lands `unknown` while the run row keeps `clean` plus the scope note.
- `TestUnscopedCleanRun_ModelRowClean` — the control: unscoped still writes
  `clean`, so the degradation is attributable to scope, not a broken status write.
- `TestPromoteConvergingToClean_SkipsRowsTaggedToAnotherModel` — reproduces the
  cross-model leak (`nginx` declared by two models), and covers both a
  `{"exit_code":0}` row and a genuinely NULL `tags` column, since `json_extract`
  over NULL must not exclude the rows the fallback exists for.

Both defect reproductions were verified to **fail against the pre-fix code**
(model row `clean` instead of `unknown`; `nginx` promoted instead of left
`converging`) and pass after. Full suite green (`CGO_ENABLED=0 go test ./...`),
`go vet ./...` clean.

The `smoke`-tagged end-to-end tests (`make smoke`) were not run — they need
docker/k3d/kubectl/mu.

## Docs

- `docs/cli-reference.md` — `--only` section documents that the scoped model
  feeds every phase and that a scoped run leaves the model row `unknown`;
  `pudl status` lists the new source of `unknown`.
- `docs/architecture-improvement-report.md` — defects 8, 9, 10 struck through
  with what was done and what is still not separable.
