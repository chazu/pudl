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
| 1.1 | `~/.pudl/schema/definitions/build.cue`, `lint.cue` | both are `brick.#Target`/`brick.#Interface` mu build+lint configs, not models | This is literally what `pudl definition list` surfaces today | ✅ **DONE (2026-06-26)** — deleted (backed up to scratchpad; outside git) |
| 1.2 | `Vault` field + `vault?:` schema field | `internal/systemmodel/systemmodel.go:38`, `internal/systemmodel/schema.cue:55`; never read anywhere; vault subsystem removed in commit `bfaaf03` (secrets now via `sealed_inputs`) | no other refs (the "tab-completion vault" was already gone) | ✅ **DONE (2026-06-26)** — removed field + schema entry; build+tests green |
| 1.3 | `entry_type='import'` / `entry_type='artifact'` taxonomy | nothing writes them (commit `ddd5030`); see §5 for the full lineage | **NOT a standalone delete** — `artifact` is entangled with Cluster A; `import` is a harmless migration default | analysed → see §5 |
| 1.4 | ~~`cmd/migrate.go`~~ | ❌ **FALSE POSITIVE** — not a stub. `cmd/identity_migrate.go:49` attaches `identityMigrateCmd` to `migrateCmd`; `pudl migrate identity` is the live command and this is its parent. | **KEEP** | rejected |
| 1.5 | ~~`datalog.Compile` / `datalog.CompileWithOptions`~~ | ❌ **MOSTLY FALSE** — `CompileWithOptions` is heavily used (`recursive.go:97,133`, `sql_eval.go:33`); only the thin bare `Compile` wrapper (`compile.go:25`) is uncalled, and it's a reasonable public convenience API. | **KEEP** | rejected |

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

**Status:** ✅ **DECIDED (2026-06-26)** — see §6 for the analysis and disposition.
Salvage 3 of 8 capabilities as ideas re-homed on the World B spine; kill the rest.

### Cluster B — pith-in-pudl: `pudl exec` + `internal/pithdriver`

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

## 6. Cluster A decision — grounded in SystemModel design intent

Analysis (2026-06-26) across pudl + the mu/ewe sibling repos: the SystemModel run
loop, the pudl↔mu contract, the ewe populate path, and a per-capability audit of
World A.

### The decisive insight

World A's **sockets + dependency graph** answer two questions — *"in what order do
these named instances apply?"* and *"how do they relate?"* The SystemModel design
**deliberately relocated both**, and recorded it:

- **Ordering → the mu DAG, not pudl.** issue-ledger **E5** (cut pure-ordering):
  "Pure cross-effect ordering is the DAG's job… mu already sequences actions by
  dependency. If you need A before B, split them into two mu DAG actions." Within a
  model, resource ordering is pushed to the plugin (k8s/kubectl).
- **Relating → Datalog, not sockets.** issue-ledger **P2**: cross-source joins are
  "pudl Datalog relations, not in-body plugin calls."
- **Capabilities → mu plugins, not interface contracts.** README two-axis rule:
  logic extends in CUE, capabilities extend as sandboxed mu plugins. No room for
  BRICK interface enforcement between definitions.

So World A is the **pre-decision model** — built before ordering/relating/capability
were assigned to the DAG, Datalog, and plugins. That makes most of it superseded by
construction, not merely old.

ewe / `#EweTarget` confirmed **irrelevant** to definitions/sockets — a populate-phase
fetch path only.

### Per-capability disposition

| World A capability | Maps to a real SystemModel gap? | Disposition |
|---|---|---|
| **Discovery / listing** | ✅ gap: no `pudl model list` (the trigger for this whole audit) | **SALVAGE THE IDEA** — build `pudl model list/show` on the existing `resolveModel` / `validator.CUEModuleLoader` (proper CUE load, `resource_type=system_model`), **not** the regex `Discoverer`. |
| **Per-definition status** | ✅ gap: run verdict not persisted; #1 convergence-roadmap item | **KEEP + WIRE IN** — `internal/database/catalog_status.go` is already catalog-native (zero World-A coupling). The run loop should *write* status (V1 §5/§8); `pudl status` is the read side. |
| **Pre-run validation** | ✅ gap: can't validate a model without running it | **REPLACE (small)** — `pudl model validate` over the real loader. |
| Socket bindings | ❌ ordering relocated to mu DAG (E5) | **KILL** |
| Dependency graph | ❌ relating relocated to Datalog (P2) | **KILL** — endorsed analog is a Datalog rule + `pudl query`, not `definition graph`. |
| Interface / BRICK enforcement | ❌ capabilities = plugins, not contracts | **KILL** (pudl-side `interface_checker`; brick.* as a mu build schema is mu's domain). |
| Standalone drift (`internal/drift.Checker`, `drift check/report`) | ❌ run loop has its own drift off `m.Desired`; Checker reads dead `entry_type='artifact'` | **KILL** — untangle: `run_drift.go` imports the `drift` pkg, so keep shared diff types (`Compare`/`FieldDiff`), drop the `Checker` + `.drift/` report store. |
| `export-actions` | ❌ render-`desired`→sources supersedes it | **KILL** |

### Collapses to

- **Build:** `pudl model list / show / validate` on the World B loader — fills the
  original gap.
- **Wire:** per-definition status into the run loop (convergence roadmap); keep
  `pudl status` as its reader (reframe its help away from World-A "definitions").
- **Delete:** `internal/definition/` (Discoverer, sockets, graph, interface_checker,
  validator); `cmd/definition*`; `cmd/drift*` + `internal/drift.Checker` (+ `.drift/`
  store); `cmd/export_actions.go`; `cmd/repo.go`'s socket-health check; and the
  now-orphaned `internal/database/catalog_artifacts.go` + `entry_type='artifact'`
  (unblocks §1.3) + the dead `GetLatestArtifact*` reads.
- **Vocabulary falls out:** "definition" stops meaning two things — the socket-instance
  sense dies; surviving "definition" = a `desired` entry / per-status unit; "model" =
  the SystemModel instance.

### Sequencing (proposed)

1. ✅ **`pudl model list/show`** (DONE, commit `fb0f8d1`) — additive; fills the gap.
2. ✅ **run-loop status persistence + `pudl model validate`** (DONE) — per-model run
   verdict written to the instance row (`modelTarget(name)`); surfaced in
   `pudl model list` (STATUS col) and `pudl status`. Includes the build-spec §5 fix
   (`ingest-manifest` exit-0 → `converging`, not a false `converged`).
3. **Delete the killed surfaces** (definition*/drift*/export-actions/repo-socket-check
   + `internal/definition`/`drift.Checker`/`catalog_artifacts`), untangling the shared
   `drift` diff types. Do last, once the replacements exist. ← NEXT

## Suggested order of attack

1. **Cluster A decision (§2)** — biggest payoff; defines what `pudl model list`
   becomes and whether the standalone drift/status/export path survives.
2. **§1 confirmed-dead** — quick cleanup pass; some falls out of (1) for free.
3. **Cluster B (§2)** — low-risk excision once decided.
4. **§3 doc/naming** — cheap, do anytime.
5. **§4 dead functions** — final mechanical sweep.
