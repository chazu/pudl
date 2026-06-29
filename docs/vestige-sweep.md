# Vestige Sweep — pudl cleanup audit

A repo-wide sweep for code/data/docs left over from prior efforts, classified by
what should happen to it: **delete**, **integrate into the ACUTE/#SystemModel
loop**, or **keep (rename/doc-fix only)**.

Working doc — we go through one item at a time. Status column tracks decisions as
we make them.

## Background: the two parallel worlds

pudl today has two overlapping ways to handle "named instances of schemas":

- **World B (LIVE spine):** the `#SystemModel` / `pudl run` **ACUTE** loop
  (populate → drift → checks → report, with optional converge). Models are found
  by `resolveModel` (`cmd/run_resolve.go`) via `_pudl.resource_type ==
  "system_model"` in the schema repo. Drift works off the model's `desired`
  entries. pudl declares state; **mu executes** (pudl never mutates — `pudl run`
  and `memory cycle` both shell out to `mu`).

- **World A (older "Phase 2: Definitions" effort):** `internal/definition/`
  scans `<schema>/definitions/*.cue` for socket-wired schema instances
  ("socket bindings", BRICK interface enforcement, a dependency graph). Surfaced
  by `pudl definition list/show/validate/graph`, and feeding the standalone
  `drift check` / `status` / `export-actions` path. This predates `pudl run`.

The triggering observation: there is **no first-class "list all #SystemModel
instances" command**. `pudl definition list` *looks* like it, but it reads the
wrong directory (World A's `definitions/`) with the wrong (socket) semantics —
and that directory currently holds only stale mu build/lint configs.

> Note on a corrected agent misread: one sweep agent claimed README/plan/CLAUDE
> are "stale" for saying the execution layer was extracted to mu, on the theory
> that `#SystemModel` *is* execution. That is wrong. pudl orchestrates; mu
> executes. "No execution layer in pudl" is accurate and should stay.

---

## 1. Confirmed dead — safe to delete (no design call needed)

| # | Item | Evidence | Notes | Status |
|---|------|----------|-------|--------|
| 1.1 | `~/.pudl/schema/definitions/build.cue`, `lint.cue` | both are `brick.#Target`/`brick.#Interface` mu build+lint configs, not models | This is literally what `pudl definition list` surfaces today | open |
| 1.2 | `Vault` field + `vault?:` schema field | `internal/systemmodel/systemmodel.go:38`, `internal/systemmodel/schema.cue:54`; never read anywhere; vault subsystem removed in commit `bfaaf03` | also a stale `vault` entry in tab-completion (`cmd` completion, commit `5a7be14`) | open |
| 1.3 | `entry_type='import'` / `entry_type='artifact'` taxonomy | nothing writes them (commit `ddd5030`); see §5 for the full lineage | **NOT a standalone delete** — `artifact` is entangled with Cluster A; `import` is a harmless migration default | analysed → see §5 |
| 1.4 | `cmd/migrate.go` | empty parent stub; real logic now lives under `identity migrate` | | open |
| 1.5 | `datalog.Compile` / `datalog.CompileWithOptions` | `internal/datalog/compile.go:25,29` flagged unreachable by `deadcode` | verify the query orchestrator isn't an intended caller before cutting | open |

---

## 2. Coherent abandoned subsystems — "integrate or kill" decisions

These are the substantive ones. Each is a self-contained subsystem that *works*
but sits parallel to the live spine.

### Cluster A — World A: `definition` + sockets + standalone drift/status/export

**What it is.** `internal/definition/` (Discoverer, `SocketBindings`, BRICK
`interface_checker.go`) feeding:
- `cmd/definition.go` + `definition_list.go` / `definition_show.go` /
  `definition_validate.go` / `definition_graph.go`
- `cmd/drift.go` / `drift_check.go` / `drift_report.go`
- `cmd/status.go`
- `cmd/export_actions.go` (the "definition → drift → `mu.json`" bridge)
- `cmd/repo.go` (socket-wiring health check)

**Why it's vestigial.** Confirmed parallel to the ACUTE spine:
- `pudl run`'s drift (`cmd/run_drift.go`) works off `m.Desired`, never the
  Discoverer.
- `export-actions` is the *old* bridge; `pudl run`'s render-`desired`→sources +
  `ingest-manifest` supersedes it.
- The `definitions/` dir it scans holds only the stale build/lint configs (1.1).

**The overloaded word.** "definition" means two different things:
- World A: a top-level socket-wired schema instance (a file in `definitions/`).
- World B / catalog: a single `desired` entry *inside* a SystemModel instance —
  and the V1 spec makes "definition" the **per-status / `--only` unit**
  (`internal/database/catalog_status.go` `UpdateStatus` keys on the `definition`
  column).

**Where "list all models" lives.** Named instances currently live in 3 places,
none of which is a clean "list models":

| Where | Discovered by | Listed by |
|-------|---------------|-----------|
| `<schema>/definitions/*.cue` (socket-wired) | World A `Discoverer` | `pudl definition list` |
| `<schema>/pudl/…` with `resource_type=system_model` | World B `resolveModel` | **nothing** |
| catalog `systemmodel.system_model` records | the catalog | `pudl list` — but only models that have **been run** |

**The decision.** Fold the useful parts (per-definition status, "list named
instances") into the ACUTE loop and delete the socket/BRICK/export machinery —
**vs** keep it as a separate path. The desired `pudl model list` command should
probably fall *out of* this decision rather than precede it.

**Status:** open — highest-value decision.

### Cluster B — pith-in-pudl: `pudl exec` + `internal/pithdriver`

**What it is.** `cmd/exec.go` ("Run a pith VM program against the pudl data
lake") + `internal/pithdriver/` (8 files: catalog / facts / schema / drift /
convert / register / schema_infer) + the external `github.com/chazu/pith`
dependency. A May-2026 experiment to query the lake via a pith concatenative VM
(commits `3143e67`, `35166c8`).

**Why it's a clean excision candidate.** Self-contained: nothing else imports
`pithdriver`. pith was subsequently absorbed into **mu** (`mu/internal/pithvm`),
and the cass-memory loop (`memory cycle`) runs by shelling **mu**, not this path.

**The decision.** Almost a pure judgment call on whether you want a pith-query
power tool in pudl at all. If not, excising it removes one command, one package,
and one external dependency.

**Analysis (resolved — removable, no functionality lost):**
- "pith datalog layer" is a misnomer. The real query layer is `internal/datalog`
  (rules→SQL, recursive fixpoint), surfaced by `pudl query` + `pkg/factstore.Query`
  with CUE-authored rules. It is **pith-free** (no pith import in `internal/datalog`
  or `pkg/`). pithdriver is a separate concatenative *scripting* surface, not
  datalog.
- Every pith word is a thin delegate to an existing method, all already exposed
  elsewhere:
  - `catalog/query`,`catalog/count` → `db.QueryEntries` (= `pudl list`)
  - `catalog/get` → `db.GetEntry`/`GetEntryByProquint` (= `pudl show`)
  - `fact/query` → `db.QueryFacts` (= `pudl facts`, `factstore.QueryFacts`)
  - `fact/assert`/`fact/retract` → `db.AddFact`/`RetractFact` (= `pudl facts add`)
  - `schema/list`,`schema/match`,`schema/infer` → `mgr.ListSchemas`,`inferrer.Infer`
  - `drift/diff` → pure in-VM list diff (no DB)
- The datalog engine is **strictly more capable** for querying (rules/joins/
  recursion) than `catalog/query`'s flat `FilterOptions`.
- The ONLY pith-exclusive thing is *programmability* (compose query + arithmetic
  + quotations in one program). It appears unused — the cass-memory loop runs on
  **mu** (`memory cycle`), not `pudl exec` — and pith-as-execution now lives in mu
  (`mu/internal/pithvm`). If programmable composition is ever wanted, it belongs
  there over the mu bridge, not as a second VM embedded in pudl.

**Blast radius (verified — nothing else depends on it):** `cmd/exec.go` +
`internal/pithdriver/` (8 files) + `examples_test.go`. Removal also drops
`github.com/chazu/pith` from `go.mod:26` **and** the `replace … => ../pith`
(`go.mod:72`) — clearing a local-replace that blocks clean external builds.
Net: ~−660 lines, −1 external dep, −1 replace directive, zero functionality lost.
Also: remove `exec`/pith pieces from tab-completion and the pith example
programs/implog references.

**Status:** ✅ **DONE (2026-06-26)** — removed `cmd/exec.go` +
`internal/pithdriver/` (8 files) + the `pith` dep/replace; cleaned the live
`guide`/`cli-reference` surfaces. 954 deletions, build+tests green. See
`implog/2026_06_26_remove_pith_exec_cluster.md`. Stale standalone pith docs
(`docs/pith-vm.md`, `docs/research/concatenative-vm.md`, the `pudl exec` mentions
in `docs/README.md` / `docs/mu-integration.md` / `docs/cass-memory-substrate-plan.md`)
left for the §3 doc-cleanup pass; implog history kept as-is.

---

## 3. Naming / doc debt (not dead code, but misleading)

| # | Item | Evidence | Fix | Status |
|---|------|----------|-----|--------|
| 3.1 | Module-name bug | `CLAUDE.md:13` and `docs/library-api.md:10` say module is `pudl`; `go.mod` says `github.com/chazu/pudl` | correct the docs | open |
| 3.2 | `cascade_validator.go` naming | code is LIVE (intended→base→catchall unification) but the name carries the removed "cascade priority" concept ("no cascade priority" per memory) | rename, don't delete — see §5 | analysed → see §5 |
| 3.3 | `skills/pudl-core/SKILL.md` | accurate as written | no action (ignore the agent that flagged it) | n/a |

---

## 4. Background noise (low priority)

~60 unreachable production functions plus a large pile of test-util dead code
(per `deadcode`). Mechanical, independent of everything above. Examples flagged:
`database.EntryExists`, `CatalogDB.GetDistinctOrigins`, `importer.New` /
`GetAvailableSchemas` / `ReloadSchemas` / `ImportFile`, several
`inference.graph` accessors, `schemaname.Parse` / `IsEquivalent`. Worth a
dedicated pass once the structural decisions (§2) are settled, since those will
delete some of this for free.

---

## 5. Deep-dive: "cascade" and the artifact/import taxonomy

Both trace back to commit **`f79297b`** ("Extract execution layer and remove
cascade priority system") — the extraction that moved glojure/executor/workflow/
model/artifact into mu.

### 5.1 Cascade — fossil name, live code

- **Was:** a *cascade priority* validation system. Schemas carried
  `cascade_priority`, `cascade_fallback`, `compliance_level`; validation walked
  candidates in priority order using that config to pick the matching schema and
  fallback.
- **Replaced by:** native CUE unification (`f79297b`). `ValidateWithCascade`
  (`internal/validator/cascade_validator.go:123-201`) now just builds a chain
  `intended → base_schema chain → catchall (pudl/core.#Item)` and tries
  `schema.Unify(data).Validate()` at each level, first success wins. No priority
  numbers, no compliance levels — CUE inheritance via `base_schema` does it.
- **Status:** code is LIVE and correct — the current validation path, called from
  `cmd/import.go` → `internal/importer/importer.go:249`. Only the **name** is a
  vestige of the removed priority system.
- **Disposition:** rename only (`CascadeValidator` → e.g. `ChainValidator` /
  `FallbackValidator`; `ValidateWithCascade`, `AddCascadeAttempt` likewise).
  Cosmetic clarity, low priority. **Do not delete.**

### 5.2 entry_type taxonomy — `import` and `artifact`

Two axes, easily conflated:
- **`entry_type`** = provenance of a catalog row (the dead axis here).
- **`resource_type`** = what the data describes, e.g. `aws.ec2.vpc` (separate,
  mostly live). The `artifact.image` *resource_type* is a different question —
  check separately; not the same as the dead `entry_type='artifact'`.

Lineage:
- The `entry_type` column was added with `DEFAULT 'import'` and NULLs backfilled
  to `'import'` (`internal/database/catalog_migrations.go:59,91`) — originally
  every row was an "import".
- Old execution model (pre-`f79297b`): definitions had **methods**; a method run
  through the executor produced an **artifact** (`entry_type='artifact'`, keyed by
  **definition + method**); drift compared declared vs. latest artifact.
- That executor/method/artifact layer was extracted to mu. mu now executes;
  results return via the mu bridge as **`observe` / `manifest` /
  `manifest-action`** (`ddd5030`: "Nothing writes import/artifact anymore").
  `pudl list --artifacts` was repointed to mean manifest+manifest-action.

Correction to §1.3's "orphaned": `internal/database/catalog_artifacts.go`
(`GetLatestArtifact` / `GetLatestArtifactByOrigin`) is still **called** by
`internal/drift/checker.go:61,74` — it's **live code reading dead data** (lookups
that always miss because nothing writes `entry_type='artifact'`). It keys on
definition **+ method** and expects artifacts: i.e. it is built on the extracted
method/artifact model.

- **Disposition:**
  - `entry_type='artifact'` + `catalog_artifacts.go` + the `method` column are
    remnants of the definition→method→artifact model, load-bearing **only** for
    the standalone (World A) drift checker. Remove them **with the Cluster A
    decision** (§2), not standalone.
  - `entry_type='import'` (migration default + NULL backfill) is harmless and
    idempotent — cosmetic cleanup at most.
  - `resource_type` `artifact` / `artifact.image` + the embedded
    `pudl/artifact/artifact.cue` (`cue_schemas.go:123`) are a separate check —
    don't conflate with the dead entry_type.

## Suggested order of attack

1. **Cluster A decision (§2)** — biggest payoff; defines what `pudl model list`
   becomes and whether the standalone drift/status/export path survives.
2. **§1 confirmed-dead** — quick cleanup pass; some falls out of (1) for free.
3. **Cluster B (§2)** — low-risk excision once decided.
4. **§3 doc/naming** — cheap, do anytime.
5. **§4 dead functions** — final mechanical sweep.
