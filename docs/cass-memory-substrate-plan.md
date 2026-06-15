# Agent-Memory Substrate Plan (cass-inspired)

Status: DRAFT / proposal. Not yet committed to `plan.md`.
Last revised after design discussion covering substrate, command shape, loop
placement, pith-target orchestration, and global rooting.

## 1. Motivation

[`cass_memory_system`](https://github.com/Dicklesworthstone/cass_memory_system)
(Dicklesworthstone) is an effective agent self-improvement loop. It is actually
two things:

- **cass** — a session archiver + full-text indexer over raw agent transcripts
  (the *episodic* tier; "ground truth").
- **cm** (cass-memory) — the memory layer *over* cass: the **ACE pipeline**
  (`Generator → Reflector → Validator → Curator`) producing a decaying,
  evidence-gated **playbook** of procedural rules.

We are rebuilding the **cm** layer. We use **neither** cass nor a reimplementation
of it — see §7.

pudl already has the hard substrate: a bitemporal, content-addressed, append-only
fact store; a Datalog engine; the `#Observation` schema with `worth`/`status`; and
TOCTOU-safe `Transact`. What it lacks is *scoring, aggregation, decay, retrieval*,
and the loop itself.

### The invariant that prevents scope creep

**pudl stores and scores; it never reflects or decides.**

- Decay is a *query* (pure, replayable), not a scheduler.
- Feedback is a *fact*, not an event handler.
- Promotion is a *Transact*, not an LLM verdict.
- Every component that needs a model lives *outside* the pudl binary.

This document covers the pudl substrate plus the orchestration that sits around it.
The ACE loop runs as **mu pith targets**, not inside pudl.

## 2. Architecture decisions (resolved)

| Question | Decision |
|---|---|
| Where does memory data live? | pudl fact store (working + procedural tiers) |
| Episodic raw transcripts? | Harness session files on disk; **not** stored in pudl, **not** cass |
| Confidence decay | Read-time view (`fact_scored`); never a mutation or scheduler |
| Reinforcement signal | `feedback` facts (append-only) |
| Aggregation (counts) | New Datalog `count`/`sum`/`max`/`min` (non-recursive only) |
| Maturity transitions | `pudl facts promote` via existing `Transact` |
| Reflector/Validator (LLM) | mu pith target calling `llm/*` or `exec/run claude`; **not** pudl, **not** nous |
| Curator (deterministic) | pudl Datalog rules + `pudl facts curate`; triggered by mu target |
| Generator (ranked injection) | Read-time harness hook → `pudl facts search` |
| Orchestration | ACE cycle = mu pith target DAG, shipped as bootstrap CUE from pudl |
| pudl ↔ mu data path | `exec/run` to the `pudl` CLI (argv form + `--json`); **not** a mu plugin |
| Global invocation | new generic `mu -C/--root <dir>`; pudl owns `~/.pudl/mu.cue` |
| Scheduling | **Out of scope.** Hook / cron / `/schedule` fire `mu build` — not ours |
| nous | Not used (separate cleanup problem) |
| dlktk | Deferred; optional upgrade path for conflict resolution |

## 3. Substrate pieces

### 3.1 Feedback as facts — `pudl facts add --relation feedback` (no engine change)

Reinforcement = append-only facts. cass `cm mark`/`cm outcome` → one relation.

Schema (`internal/importer/bootstrap/pudl/nous/nous.cue`):

```cue
#Feedback: {
	target:   string                          // fact/rule ID the feedback is about
	verdict:  "helpful" | "harmful" | "neutral"
	outcome?: "success" | "failure"
	source:   string                          // agent name
	note?:    string
}
```

Content-addressed: same agent + target + verdict = dedup; different agents =
distinct facts = corroboration preserved (same model as observations). The "4×
harmful weight" cass uses is a *scoring* choice → lives in the decay view (§3.3),
not in ingest.

### 3.2 Datalog aggregation — `count`/`sum`/`max`/`min` (engine work, medium)

Compiler today emits `SELECT DISTINCT ... json_extract self-joins`, no `GROUP BY`.
General engine feature, valuable beyond cass.

Type extension (`internal/datalog/types.go`):

```go
type Term struct {
	Variable string
	Value    interface{}
	Agg      string // "count" | "sum" | "max" | "min" — head-only
}
```

Compiler (`compile.go`): head vars with `Agg==""` → `GROUP BY` columns; vars with
`Agg` set → aggregate in SELECT, drop `DISTINCT`.

Scope guard: **non-recursive rules only**. Aggregation inside a fixpoint needs
stratification — reject in `partition.go` with a clear error.

```
harmful_count{target:$T, n:count($S)} :- feedback{target:$T, verdict:"harmful", source:$S}
```

### 3.3 Decay as a query, never a mutation (reuse EDB-view trick, medium)

Computed at read time, never written back — store stays replayable; "what did we
believe at T" still holds. The compiler can't emit computed arithmetic, so **don't
touch it** — reuse the `catalog_entry_edb` pattern (`builtin_edb.go` +
`TableOverrides`, native columns).

New view `current_facts_scored` (`internal/database/`):

```sql
CREATE VIEW current_facts_scored AS
SELECT id, relation, source,
       (unixepoch() - valid_start)               AS age_seconds,
       json_extract(args,'$.worth')              AS worth,
       json_extract(args,'$.worth') * pow(0.5,
         (unixepoch() - valid_start) / 7776000.0) AS decayed_worth  -- 90d half-life
FROM current_facts;
```

PROBE FIRST: `pow()`/`exp()` need `SQLITE_ENABLE_MATH_FUNCTIONS` in
modernc.org/sqlite (`SELECT pow(2,3)`). If absent: expose `age_seconds` + `worth`
raw and threshold linearly in a rule, or register a Go scalar on the driver.

Register `current_facts_scored` as built-in EDB relation `fact_scored` (reserved,
join-only, same wiring as `catalog_entry`):

```
live_rule{id:$I, w:$W} :- fact_scored{id:$I, relation:"playbook", decayed_worth:$W}, $W > 0.25
```

This is the single most important line: decay is a view over truth, not a scheduled
writer. Keeps pudl a bitemporal truth store, not a fuzzy ranking engine.

### 3.4 Maturity state machine — `pudl facts promote` (reuse Transact, easy)

`#Observation.status` already has `raw → reviewed → promoted | rejected`. Build the
*transition*, not the *judgment*.

```
pudl facts promote <id> --to reviewed
pudl facts promote <id> --to promoted --rule "<rule-id>"   # sets promotedTo pointer
```

`Transact`: read current status, validate legal transition, append updated version.
Illegal jump (`raw→promoted`) → error inside the tx → rollback.

pudl flips the flag and records `promotedTo`. It does **not** decide whether to
promote and does **not** synthesize the rule — that is the caller's (the mu target's
LLM stage) job. pudl supplies the atomic transition + the evidence query (§3.2).

### 3.5 FTS5 over Args — `pudl facts search` (optional, lower priority)

Keyword recall (cass's `cm context` keyword half). Skip embeddings — vectors pull in
model deps, which is mu/agent territory, not pudl's.

FTS5 virtual table mirroring `facts.args`, synced in `AddFact`/`RetractFact`.
PROBE FIRST: FTS5 availability in the modernc build. Defer if it complicates the
build.

## 4. Command consolidation — one right way to add data

Constraint: agents must have ONE obvious way to write each kind of data.

Decisions:

- **One canonical low-level write:** `pudl facts add --relation R --args JSON`,
  schema-validated against the relation's registered CUE `#Schema`. This is the
  single door for feedback, playbook bullets, diary entries, and any future
  relation. The substrate does **not** sprout one verb per relation.
- **`pudl observe` → `pudl facts observe`** — kept as ergonomic sugar for
  `facts add --relation observation`, explicitly documented as such.
- **Feedback gets no top-level verb** → `pudl facts add --relation feedback`.
- **`pudl facts promote`** — a *transition* verb, legitimately distinct from add.
- **`pudl facts search`** — FTS (§3.5).
- **mu-bridge commands → `pudl mu` namespace:** `pudl mu export-actions`,
  `pudl mu ingest-observe` (audit `ingest-manifest` — likely mu too). Plain
  `pudl export` (data export) stays separate — it is not a mu bridge.

Net new agent-facing surface: `facts add`, `facts observe`, `facts promote`,
`facts search`, `facts curate` — all under the existing `facts` group, plus a `mu`
namespace move. **Zero new top-level verbs.**

`ingest-observe`/`import` remain data-lake doors, conceptually separate from the
fact/agent-memory door. `prime`/`guide` MUST draw the line (see §11).

## 5. Loop placement — where each ACE stage runs

The ACE loop is **not** pudl. Two stages genuinely require an LLM (verified against
the cass README):

| Stage | LLM? | Runs as |
|---|---|---|
| Generator (keyword extract + rank + inject) | No | read-time harness hook → `pudl facts search` |
| Reflector (session → candidate rules) | **Yes** | mu pith target, `llm/*` or `exec/run claude` |
| Validator (evidence gate + accept/reject) | **Yes** | mu pith target; evidence = `pudl query harmful_count`; verdict reasoned by LLM |
| Curator (dedup / conflict / promote) | No | mu pith target → `pudl facts curate` (Datalog) |

The LLM-bearing stages call models **from the mu target**, never inside pudl. They
reach pudl as a subprocess (`exec/run`). cass's core uses external API keys; the
agent-native path (`exec/run claude`) reuses the harness subscription instead — our
default (§8).

Explicitly **not** in nous (separate cleanup problem) and **not** a daemon —
triggering is event/lazy, never a long-running service.

## 6. Substrate sequencing

| Phase | Pieces | Engine change | Unlocks |
|---|---|---|---|
| A | 3.1 Feedback + 3.4 Promote | none — schema + CLI + Transact | record signal, move maturity |
| B | 3.2 Aggregation | compiler GROUP BY | corroboration counts, evidence gate |
| C | 3.3 Decay view | view + EDB wiring (probe `pow()`) | recency-weighted recall |
| D | 3.5 FTS | virtual table (probe FTS5) | keyword recall |

Phase A ships now, zero engine risk. B is the one piece of genuine engine work and
pays off beyond cass. C carries the one real unknown (math functions).

## 7. Episodic tier — use neither cass nor a reimplementation

The episodic raw-transcript tier already exists: the **harness writes session files
to disk** (e.g. `~/.claude/projects/.../*.jsonl`). The Reflector reads those
directly, exactly as cm reads cass logs.

- **Don't depend on cass** — external dependency, another moving piece.
- **Don't reimplement it in pudl** — pudl is structured knowledge, not a log sink;
  raw transcripts as facts = scope bloat and ugly blobs.
- pudl holds **working + procedural tiers only** (observations, feedback, playbook).

If cross-session FTS over raw transcripts is ever needed, build a thin separate
indexer then. YAGNI for v1.

## 8. Orchestration — ACE as a pith target DAG

pith is a concatenative VM **shared by mu and pudl** (`github.com/chazu/pith`;
`pudl exec` and mu targets both run it). mu targets can be authored entirely in CUE
with inline pith `plan` / `transform` / action `body` — no plugin binary. Proven by
`examples/pith-e2e/` (mu's dag tests green).

### The cycle (shipped by pudl as bootstrap CUE)

```cue
// pudl/memory — bootstrap CUE, loaded into a mu workspace
targets: [
  { target: "//memory:reflect"           // LLM
    config: { lookback_days: 1 }
    plan: [ /* body: read ~/.claude session files, distill, then
               exec/run ["pudl","facts","add","--relation","observation",...,"--json"] */ ] },
  { target: "//memory:validate"           // LLM, gated by deterministic query
    deps: ["//memory:reflect"]
    plan: [ /* body: exec/run ["pudl","query","harmful_count",...]; if evidence ok,
               llm/complete deep-validate; exec/run ["pudl","facts","promote",...] */ ] },
  { target: "//memory:curate"             // NO LLM — pure
    deps: ["//memory:validate"]
    plan: [ /* body: exec/run ["pudl","facts","curate"] (Datalog dedup/conflict/promote) */ ] },
  { target: "//memory:cycle" deps: ["//memory:curate"] },
]
```

`mu build //memory:cycle` runs the DAG (reflect → validate → curate). The Generator
stays a separate **read-time hook** (`pudl facts search` at session start), not part
of this build graph. Decay stays the lazy `fact_scored` view.

### Data path: `exec/run` to the pudl CLI — NOT a mu plugin

A mu plugin responds to `plan`/`observe` over NDJSON *in its own process*. For
`reflect`/`validate` the work IS the LLM call — a pudl plugin would run reflection
**inside pudl**, detonating the §1 invariant. It also adds protocol-version coupling
for deterministic ops that a subprocess already handles. So:

- mu targets reach pudl via **`exec/run ["pudl",...]`** — argv form (no shell
  quoting), `--json` for structured stdout (parse with pith `format/json`). ~90% of
  plugin structure, 0% of the coupling.
- The LLM call lives in the target (`llm/*` / `exec/run claude`), never in pudl.
- pudl never imports mu; mu never links pudl.

Considered and rejected: mu importing pudl's `internal/pithdriver` (catalog/fact/
schema words) for *in-process* pudl access. The pith VM runs in mu's process during
a build; giving it pudl drivers forces mu to import pudl as a Go dependency. The
subprocess boundary is cleaner. (pudl's in-process pith drivers remain for
standalone `pudl exec` only.)

### LLM convenience word

Add `llm/complete` to pith (shared by mu + pudl): wrap `http/post` to a configured
provider with the API key as a **sealed input** (harness-independent). Keep
`exec/run claude` as the agent-native escape hatch (reuse the existing harness
subscription, no key to manage) — **our default for v1**.

## 9. Global invocation — generic `mu -C/--root <dir>`

mu today walks **up** from cwd to the nearest `mu.cue` (`internal/config/loader.go`);
there is **no** home/global fallback (the `~/.mu/` references are the *cache*). The
memory cycle is not project-scoped — it operates on the global pudl store
(`~/.pudl`).

Key reframe: this is **not** a new *target type*. The cycle is an ordinary mu target
that happens to operate on global state. The only missing primitive is the ability to
root mu somewhere other than cwd.

Decision — add a small, generic flag:

```
mu -C ~/.pudl build //memory:cycle
```

- ~20 lines: when `-C` is set, skip cwd discovery and root there.
- Generic ("run mu from anywhere, root explicit"), independently useful — **not**
  pudl-specific.
- pudl owns `~/.pudl/mu.cue` defining the ACE targets, and a wrapper:

```
pudl memory cycle   →   mu -C ~/.pudl build //memory:cycle
```

Free symmetry:

- **global memory:** `mu -C ~/.pudl build //memory:cycle` (operates on `~/.pudl`)
- **repo memory:** `mu build //memory:cycle` from inside a repo — normal upward
  discovery finds `.pudl/mu.cue`. No special casing.

Both are ordinary targets at different roots. No global-target concept required.

**Not pudl-only:** `-C` is a generic mu primitive; pudl is just the first tenant
pointing it at `~/.pudl`. Coupling mu's core to pudl is the exact cross-coupling we
avoid everywhere else.

### Caching / hermeticity caveat

mu's action cache hashes inputs (`hash(canonical(body) + input_digests)`). The memory
targets read **mutable global state** (`~/.claude` session files, the `~/.pudl`
store) that is not a declared, content-addressed input. They are effectively
non-hermetic and should **not** be cached (always run). VERIFY whether mu supports a
per-target "no-cache / always-run" flag; if not, that is a small prerequisite mu
change. This caveat applies regardless of `-C` vs a future home workspace.

## 10. Future enhancement — a "proper" global-target feature (mu home workspace)

`-C` covers 100% of the memory-loop need. A richer feature — a **mu home workspace**
(`~/.mu/mu.cue` or `$XDG_CONFIG_HOME/mu/`) whose targets are *always in scope*,
merged with the project's — is deferred until a *second* global-target use case
appears. If built, keep it **generic** (any tool drops targets there; pudl is one
tenant), never pudl-only.

Challenges to solve before that is worth building:

1. **Merge semantics** — how do home targets and project targets combine? Union, or
   project shadows home? Define precedence explicitly.
2. **Namespace collisions** — `//memory:cycle` in both home and project. Needs a
   reserved root or an explicit address sigil (e.g. `@home//memory:cycle`), which
   means target-resolution parser changes.
3. **Load order / cost** — always load the home workspace (cost on every invocation),
   or only under a flag/env? Always-on is convenient but adds CUE evaluation to every
   `mu` call.
4. **CUE unification scope** — are home + project unified into one `cue.Value`, or
   evaluated separately? Package/identifier collisions across two `package mu` files
   must be handled.
5. **Plugin / toolchain resolution root** — relative to home root or project root?
   A home target referencing a relative plugin path is ambiguous.
6. **Hermeticity / cache** — home targets reaching global mutable state break the
   content-addressed cache model (same issue as §9, amplified). Likely need a
   first-class "non-hermetic target" notion.
7. **Security** — a home workspace auto-loaded on every build is implicit code
   execution from a global file. Needs a trust/opt-in story.

## 11. prime / guide clarity (required)

`pudl prime` and `pudl guide` must draw three hard lines so agents never guess how to
write data:

- **Assert a fact / record memory →** `pudl facts add` (or sugar `pudl facts observe`).
- **Import data into the lake →** `pudl import`.
- **mu bridge (drift → actions, observe results) →** `pudl mu …`.

And document the read path: recall ranked playbook via `pudl facts search` /
`fact_scored`; correct via `pudl facts retract` / `invalidate`; advance maturity via
`pudl facts promote`.

## 12. pith status (corrected)

pith ↔ pudl/mu integration is **essentially complete** (per the project's own
integration record): Q1–Q7 resolved, 915 pudl tests + 428 mu tests passing. pith core
has 127 tests; CUE extraction, trace mode, single-quote string literals, op-index
errors, field refs (`ctx.*` via `SetContext`), arithmetic, `group-by`/`flatten` all
landed. mu exec vocab: `http/get`, `http/post`, `exec/run`, `exec/shell`,
`cas/store`, `cas/fetch`, `format/json`, `format/compact`. pudl pith drivers:
`catalog/*`, `fact/*` (incl. `fact/assert`, `fact/retract`), `schema/*`, `drift/diff`.

Remaining pith work (NOT "open integration questions"):

1. **Publish pith to GitHub, remove `replace` directives** (the one stated leftover).
2. **`llm/*` convenience words** — new work for this project (§8). `http/post` exists;
   `llm/complete` is sugar over it.
3. **`internal/pithvm` (mu's registration shim) has no direct unit tests** — covered
   only transitively via mu's dag tests. Add focused tests for the plan/transform/
   execute driver registration layer.

## 13. Master worklist

1. **pith close-out:** publish module + drop `replace`; add `llm/complete` word; add
   `internal/pithvm` unit tests.
2. **mu:** generic `-C/--root` flag. (Per-target no-cache NOT needed — build-level
   `--no-cache` on the dedicated `~/.pudl` workspace covers it; see §15.)
3. **pudl substrate Phase A — DONE (2026-06-14):** `#Feedback` schema; `facts add`
   (JSON-shape + auto strict validation of known relations via embedded-def
   unification + opt-in `--schema` + `--no-validate`); `facts observe` (moved under
   `facts`, now validated); `facts promote` (WithFactTx state machine); `pudl mu`
   namespace move; `prime`/`guide` updated with the three write doors (§11 done).
   Build + full suite green. See `implog/2026_06_14_cass_memory_phase_a.md`.
4. **pudl substrate Phase B — DONE (2026-06-14):** Datalog aggregation
   (`count`/`sum`/`max`/`min`) via `agg($Var)` head syntax → SQL `GROUP BY`;
   non-recursive only (recursive aggregation rejected). Tests + CLI e2e green. See
   `implog/2026_06_14_cass_memory_phase_b_aggregation.md`.
5. **pudl substrate Phase C — DONE (2026-06-14):** `fact_scored_edb` view (join of
   current_facts+facts; age + 90-day half-life `decayed_worth` via `pow()`) +
   `fact_scored` join-only EDB relation. Tests + CLI e2e green. See
   `implog/2026_06_14_cass_memory_phase_c_decay_view.md`.
   **Follow-up DONE:** Datalog comparison operators (`>`,`<`,`>=`,`<=`,`!=`) so the
   recall gate `decayed_worth > X` lives in the rule. See
   `implog/2026_06_14_cass_memory_comparison_operators.md`.
6. **pudl substrate Phase D — DONE (2026-06-14):** FTS5 `current_facts_fts` index
   (values-only, synced at the current_facts mutation points) + `pudl facts search`
   (ranked, relation filter, limit). Keyword only; embeddings out of scope. Tests +
   CLI e2e green. See `implog/2026_06_14_cass_memory_phase_d_fts.md`.
7. **Curator:** `pudl facts curate` (Datalog dedup/conflict/promote rules).
8. **Orchestration:** ship `pudl/memory/*.cue` ACE targets; `pudl memory cycle`
   wrapper; Generator read-time hook template (Claude Code first).
9. **Docs:** ~~update `prime`/`guide` (§11)~~ DONE; still TODO: update VISION.md
   (loop now external pith targets, not "nous medium loop").

## 14. Deferred / open

- **dlktk** — natural home for conflict resolution if it becomes a graph problem
  (preference cycle-safety already exists). Not v1.
- **Embeddings / semantic recall** — model deps; out of pudl. Keyword FTS only.
- **Per-harness hook adapters** — Claude Code first; Cursor/Codex later.
- **Option B standalone reflect runner** (own API key) — only if harness-independent
  operation is needed; keep out of the pudl binary.
- **mu home workspace** (§10) — deferred with challenges enumerated.
- **mu non-hermetic/always-run target flag** — verify existence (§9).

## 15. Probes — RUN, no blockers

All three executed against the live modernc/mu builds. Result: **not blocked.**

1. **Math funcs (decay, §3.3):** ✅ `pow(2,3)=8`, `exp(0)=1`, `unixepoch()` all work
   in the modernc build. Decay view buildable exactly as designed — no fallback
   needed.
2. **FTS5 (§3.5):** ✅ `CREATE VIRTUAL TABLE ... USING fts5` + `MATCH` work. Keyword
   `facts search` buildable.
3. **mu per-target no-cache (§8/§9):** ⚠️→✅ No per-target flag exists (`Target`
   struct has no cache field). BUT a build-level `--no-cache` exists
   (`cmd/mu/build.go:24`). Because `~/.pudl/mu.cue` is a **dedicated** workspace
   (memory targets only), global no-cache loses nothing — the `pudl memory cycle`
   wrapper always passes it: `mu -C ~/.pudl build //memory:cycle --no-cache`. So the
   hermeticity concern dissolves for v1; a per-target `always-run` flag becomes a
   nice-to-have, not a prerequisite.

Only genuine engine build remaining: Datalog aggregation (§3.2). Everything else is
additive on verified-green primitives.
