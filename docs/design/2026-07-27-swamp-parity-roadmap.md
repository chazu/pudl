# Design: closing the utility gap with swamp

**Date:** 2026-07-27
**Scope:** pudl + mu, jointly
**Status:** execution plan, hardened after adversarial review (§8) and the
follow-up domain grilling in §10. The pre-value-wiring boundary is R1/R3/R4/R5/
R8/R9/R10, including the real mu-side Kubernetes inventory work. R6 (cross-model
value wiring) remains explicitly deferred until the stocktake gate in §13.
**Evidence:** `swamp 20260723.231121.0`, the `~/dev/loosh/dev` repo (a live
swamp + mu deployment), and the swamp agent skill tree at
`~/.claude/skills/swamp/` (SKILL.md + 12 per-primitive guides + ~50 references)

**Successor:** This roadmap intentionally stopped before R6. The accepted
follow-on contract is
[`2026-07-28-cross-resource-value-wiring.md`](2026-07-28-cross-resource-value-wiring.md),
which includes plain CUE elaboration and full v1 mu sealed I/O integration.
References to R6 as deferred below describe this completed roadmap's historical
delivery boundary, not the current project plan.

## 1. What swamp is

A repo-scoped automation runtime with a package registry behind it. State lives
in `.swamp/` (SQLite catalogs, bundle caches, run tracker, workflow runs);
authored artifacts live in git-tracked `models/`, `workflows/`, `extensions/`,
`vaults/`, `grants/`.

Five primitives:

| Primitive | Shape | Notes |
|---|---|---|
| **Model type** | TypeScript, `export const model` with zod `globalArguments`, declared `resources` (output schema + lifetime + GC count), named `methods` | CalVer-versioned (`2026.06.10.1`), published to swamp-club |
| **Model instance** | YAML, swamp-assigned UUID, at `models/@collective/type/<uuid>.yaml` | In loosh these are ~10 lines: `type`, `name`, `globalArguments` |
| **Data** | versioned artifacts written by method runs | queried with CEL, tagged, projected via `--select`, GC'd by policy |
| **Workflow** | YAML DAG: jobs → steps, each step a `model_method` or nested workflow | `dependsOn: [{step, condition:{type: succeeded\|completed\|failed}}]`, `weight`, `allowFailure`, approvals, resume |
| **Report** | extension that runs **automatically** after every method run and workflow step | emits markdown + JSON, **including when the step failed** |

Supporting layers: **vaults** (pluggable secret backends with audit trail and
migrate), **datastores** (pluggable state backends with locking + sync — the
mechanism by which a solo repo becomes a team repo), **drivers** + a **worker**
pool for remote execution, `swamp serve` (WebSocket API, TLS, token/OAuth auth,
grant-based authz), and a **registry** with keyword/platform/content-type
search, a 12-factor quality rubric, OSV dependency auditing, trust collectives,
and yank/deprecate.

### 1.1 What a real deployment looks like

`~/dev/loosh/dev` has five model instances (four `@swamp/kubernetes/*`, one
`@swamp/ssh`, one `@webframp/terraform`) totalling about 60 lines of YAML, and
two workflows (`hermes-health`, `hermes-update`) totalling about 130 lines. All
of the actual capability — kubectl invocation, SSH over Tailscale, terraform
state reading — came from the registry. Nothing was authored locally except
configuration and orchestration.

That ratio is the thing to notice.

## 2. Functional comparison

| Axis | swamp | pudl + mu |
|---|---|---|
| Unit of automation | model type + method — small, runnable alone | `#SystemModel` — only `name` + `populate` are required, so it is small too; the barrier is authoring, not size |
| Ad-hoc execution | `swamp model @type method run <m> <n> --input k=v`, **no files authored** | `pudl run --populate plugin:<n> --input k=v`, observe-only, no model file |
| Instance authoring | `swamp model create` scaffolds, human/agent edits the returned path | `pudl model new` scaffolds; `rule new` and `model populator new` cover adjacent authoring seams |
| Capability discovery | `type search`, `type describe --json` (methods + argument schemas), registry search | `mu plugin info <n> --json` / `list --discover` return capabilities + `config_schema`; `pudl help --json` / `model describe --json` expose the PUDL contract |
| Distribution | published registry, versioned, quality-scored, trust model | `mu plugin push` / `list --remote` over OCI; `pudl module add` over the CUE registry. Both exist; neither is documented as a pair |
| Query language | CEL — predicate over one record | **Datalog** — joins, recursion, over facts × catalog |
| History | version chain + GC policy | **bitemporal** (valid-time × transaction-time), append-only |
| Cross-unit value wiring | CEL `data.latest("m","d").attributes.f`, lazily resolved | `depends_on` is ordering only; value interpolation explicitly parked |
| Orchestration | DAG with conditions, approvals, resume, foreach, nesting | mu build graph; `pudl run` is a fixed 5-phase pipeline with request-level converge approval/resume |
| Post-run diagnostics | auto reports, `report get --json`, `run doctor --fix`, `doctor <subsystem>` | durable `pudl run report [<run-id>] --json`, `pudl doctor`, mu logs |
| Work caching | none — every method re-runs | **mu CAS** — content-addressed, skips unchanged work |
| Runtime | embedded Deno at `~/.swamp/deno`, npm deps inlined into bundles | single static Go binary, no CGo |
| Concurrency | per-instance file lock held for the whole method | mu graph parallelism |
| Team | datastore sync, serve, grants, tokens, worker pool | single-writer by design |
| Agent memory | none | `observe` → facts → maturity curation → SessionStart injection |

The right-hand column reflects the delivered pre-value-wiring slices below; the
original baseline and its evidence remain recorded in the R1/R3 findings.

## 3. Diagnosis

Two findings. The second follows from the first.

**3.1 — baseline acquisition was the largest gap.** `@swamp/kubernetes/*` is
15 types in the box: point at a cluster, get typed, versioned, queryable data.
Before this roadmap, pudl's answer to "get me my pods" was "write an ewe
populator." The delivered mu inventory path plus `pudl/k8s.#Resource` now closes
that first-use path for the bounded inventory contract; Terraform remains a
text-shaped observer and is intentionally outside this slice.

**3.2 — the barrier is authoring, not unit size.** *An earlier draft claimed
`#SystemModel` was too large a unit. That was wrong, and the review caught it.*
`internal/systemmodel/schema.cue` requires only `name` and `populate`;
`depends_on`, `plugins`, `schema`, `relations`, `checks`, `desired`, `converge`
and `freshness` are all optional, and `pudl run` is observe-only by default. A
working model is four lines:

```cue
name: "pods"
populate: {plugin: "k8s", input: {...}, differential: false}
```

At baseline those four lines had to be hand-authored in CUE, in the right
package, with the right import, registered in a schema repo, before anything
ran. The delivered scaffold and `--input k=v` paths now skip that ceremony for
first use while preserving `#SystemModel` as the durable unit. The gap was
**scaffolding and an ad-hoc entry point** (R3), not a new primitive.

## 4. What not to trade away

These are real advantages, not consolation prizes. Every recommendation below is
constrained to preserve them.

- **Datalog is structurally more expressive than CEL.** CEL is a predicate over a
  single record. It cannot express `depends_transitive`, `impacted_by`, or
  `cyclic`. Swamp's answer to a recursive question is "write a report extension
  in TypeScript."
- **Bitemporal ≠ versioned.** Swamp GCs old versions by policy. "What did we
  believe on 2026-04-01 about the state as of 2026-03-01" is a pudl-only
  question.
- **Schema inference plus CUE unification.** Swamp requires a hand-written zod
  schema per type, and zod does not unify. pudl infers, then lets you tighten,
  and base-schema chains compose.
- **mu's CAS is a real build system.** Swamp re-runs everything and holds a
  per-instance lock for the entire method duration — which has leaked into
  user-facing guidance ("prefer fan-out methods over loops… parallel calls
  contend on the per-model lock, causing timeouts") and a documented
  "disposable instances" workaround pattern.
- **One static binary.** Swamp ships an embedded Deno, inlines npm packages into
  extension bundles (their own rule 7: `deno.lock` and `package.json` do *not*
  cover extension model dependencies), and has a documented
  `rm -rf .swamp/<type>-bundles/` staleness failure mode.
- **The agent-memory loop** (`observe` → facts → `facts curate` → `memory
  context` at SessionStart) has no swamp analogue at all.

## 5. Recommendations

Ten items were drafted in three tiers. After review (§8), **R2 and R7 are cut,
R6 is deferred, and R5 and R9 are substantially reduced.** The cut entries are
kept in place with their reasons rather than deleted — what was rejected and why
is the more useful record. §7 carries the resulting order, which is flat rather
than tiered.

### Group A — the day-one gap (as drafted: "Tier 1")

#### R1 — Standard library of observers (RESCOPED)

**Problem.** pudl is empty on day one. Swamp is not.

**The observers should be `#PluginObserve`, not `#EweTarget`.** Four structural
reasons:

1. **`eweSource` is a repo-relative path.** `models.#GithubChazu` has
   `eweSource: "github-chazu/populate.cue"`, resolved under the repo's
   `populators/` directory and installed by `pudl model populator add`. A
   standard library cannot ship a repo-relative path — every consuming repo
   would need its own copy, which is the opposite of a standard library.
   `#PluginDef` already carries four forms — `command:`, `script:`, `digest:`
   (from the `~/.mu/plugins` CAS), `url:` + `sha256:` — of which the last two are
   distribution mechanisms. `eweSource` is not one.
2. **Converge is plugin-only.** `#PluginPlan` is `plugin:` + `input:`.
   Ewe-populate plus plugin-converge means two authoring mechanisms for one
   system, where `k8s` and `terraform` each serve both directions from a single
   plugin.
3. **`differential` exists only on `#PluginObserve`** — the path where the
   observer reads `desired` as sources and reports per-resource exists/matches.
   Ewe targets are inventory-only.
4. **The quoted-`"_schema"` convention is a per-record runtime burden** on the
   populator author, with a CUE hazard documented in `systemmodel.cue` itself. A
   standard library observer should declare its schema statically.

`#EweTarget` stays as the escape hatch for bespoke API fetches with no plugin —
the GitHub/GitLab case, which is exactly what `models.#GithubChazu` is today. If
GitHub joins the standard library, it becomes a mu plugin.

**But the baseline observers did not exist in the shape an earlier draft
claimed.** That draft asserted all four candidate plugins emit `_schema`-tagged
inventory records. Verified against `~/dev/go/mu/plugins/`, only two did before
this slice:

| mu plugin | Declared capabilities | Observe output | pudl package |
|---|---|---|---|
| `host` | discover, observe | six `_schema` types via `gather.sh` (`linux.host`, `.package`, `.service`, `.filesystem`, `.user`, `.network_interface`) | **`pudl/linux` exists, 6 schemas — all six map correctly. Works end to end today.** |
| `aws` | discover, observe | `_schema` on `aws.ec2.instance` / `.vpc` / `.subnet` (`plugin.bb:98-101`) | `pudl/aws` existed, 16 schemas, VPC/networking-centric; baseline lacked `#Instance`, now added |
| `k8s` | discover, plan, observe | Differential remains `{"current":{"resources":[{resource, exists, matches, diff}]}}`; explicit `inventory` now returns `_schema: "k8s.resource"` records | `pudl/k8s.#Resource` open inventory envelope |
| `terraform` | discover, plan, observe | `{"current":{"has_changes": bool, "plan_output": "<text blob>"}}` (`plugin.bb:227-261`). No records. | n/a — one string field, nothing to schematize |

Two corrections worth recording separately:

- **No mu plugin declares an `apply` capability.** `k8s` and `terraform` both
  declare `["discover" "plan" "observe"]`; apply is an *action emitted inside
  plan output* that mu executes. Reason 2 above survives — one plugin still
  serves both directions — but "observe + plan + apply from a single plugin" was
  the wrong phrasing.
- **`plugins/k8s/GUIDE.md:54` is wrong.** It states 'Records include `_schema`
  "k8s.<kind>" (e.g. "k8s.deployment")'. `handle-observe` emits no such field.
  This doc/implementation mismatch is what produced the error above, and it
  should be fixed in mu regardless of anything in this document.

**What was actually broken at baseline.** `internal/mubridge/ingest.go:453`
`resourceTypeToSchema` maps a `_schema` tag to a CUE ref by splitting on the
*first* dot and upper-casing one character:

| Plugin emits | Maps to | Reality |
|---|---|---|
| `linux.host` | `pudl/linux.#Host` | ✅ resolves |
| `linux.network_interface` | `pudl/linux.#NetworkInterface` | ✅ resolves — the underscore loop at `ingest.go:463-470` handles it |
| `aws.ec2.vpc` | `pudl/aws.#Ec2.vpc` | ❌ **invalid CUE ref** — `SplitN(_, 2)` leaves `ec2.vpc` as the definition name |
| (`aws.vpc` would give) | `pudl/aws.#Vpc` | ❌ shipped schema is `pudl/aws.#VPC` |

`ingestObserveRecord` (`ingest.go:335`, call at `348-351`) takes that result
**unconditionally** — no existence check, no fall-through to inference. A record
whose tag does not map gets a `schema_ref` pointing at nothing.

**Revised proposal and delivered result.** R1 is much smaller than the earlier draft claimed, and
splits into one item that is ready now and one that is real mu work:

1. **Fix the routing contract** *(ready now, ~a day)*. Handle the multi-dot case
   and the acronym case, and add an existence check so an unresolvable ref falls
   back to inference rather than being written as a dangling `schema_ref`.
   Before building anything new here, check whether this unifies with the
   existing unresolved-ref machinery — `cmd/reclassify.go`,
   `database.ListUnresolvedItemSchemas`, `internal/muschemas` — which covers the
   parallel `<module>@<version>` plugin-output path and is currently a
   status-reporting stub. Two mechanisms for "this schema ref does not resolve"
   is one too many; finishing the stub may be the cheaper path.
2. **Add `#Instance` to `pudl/aws`**, matching what the observer emits.
3. **A k8s inventory observe path in mu** *(separate item, real cost)*. This is
   what the "point at a cluster, get pods" success measure actually requires, and
   no amount of pudl-side schema authoring substitutes for it. It should be
   filed against mu with its own estimate, not smuggled in under "ship a standard
   library."

`pudl/host` and `pudl/terraform` are **not** to be authored: `pudl/linux`
already covers host, and Terraform still emits a text-shaped observer result.
Once the mu inventory path exists, however, PUDL carries a minimal open
`pudl/k8s.#Resource` envelope so those records have a stable catalog schema.

**Why this still comes first.** Items 1 and 2 are unblocked defect fixes on the
one plugin pair that already works. Item 3 is the honest price of the
kubernetes story.

**Success measure.** Items 1–2: `mu observe --json //host/... | pudl mu
ingest-observe` and the `aws` observer both land records with resolving schema refs, and
no record in the catalog carries a dangling one. Item 3 restores the original
measure (a kubeconfig produces queryable pod records with no CUE authored).

#### R2 — ~~Add a primitive below `#SystemModel`~~ (CUT)

**Cut by review.** The premise was false. §3.2 now records the correction:
`#SystemModel` requires only `name` and `populate`, so the proposed `#Source`
(`{name, plugin, input, schema?}`) is structurally identical to a minimal
`#SystemModel` that is already legal today. The recommendation would have added
a CUE definition, two CLI nouns, and a new file location to obtain a shape the
schema already permits.

It was also internally incoherent: `#Source` could not express either arm it
claimed to replace. `#PluginObserve` carries `differential`, which routes
inventory versus differential drift and is read by nothing outside the ACUTE
loop — so adding it violated R2's own minimality constraint, and omitting it
made the re-expression impossible. `#EweTarget` carries `eweSource`, `outputs`,
`network`, `impure`, `sealed_inputs`, `sealed_input_modes`, none of which
`#Source` had.

**What survives.** The ad-hoc entry point was the only genuinely missing thing,
and it does not need a new type. It becomes a flag on the existing command, in
R3.

#### R3 — Scaffolding, and one ad-hoc entry point

**Problem.** Agents hand-author CUE and get it wrong in ways the schema's own
comments predict. `internal/systemmodel/schema.cue` documents the quoted
`"_schema"` trap — a bare `_schema` is a hidden CUE field, `json.Marshal` drops
it, and the routing tag silently never reaches the records file. That is a
hand-authoring bug class a generator eliminates outright.

**Proposal.**

| Command | Emits |
|---|---|
| `pudl model new <name> --populate plugin:<n>` | the four-line `#SystemModel` from §3.2 — correct package, correct import, populate arm filled in |
| `pudl rule new <name>` | `#Rule` skeleton with head/body and `$`-var conventions |
| `pudl model populator new <model>` | ewe populator stub emitting the quoted-`"_schema"` convention correctly |

All print the written path (and honour `--json`, returning `{path, name}`), so
the agent workflow is scaffold-then-edit.

**Plus one ad-hoc entry point** — the piece that survives from the cut R2, as a
flag rather than a new noun:

```
pudl run --populate plugin:k8s --input context=foo
```

Constructs the model in memory, runs it observe-only, writes no file. This is
what makes the first 20 seconds work, and it reuses the entire existing run
path.

The rule and populator generators are intentionally thin: they produce the
existing CUE shapes and write paths; they do not add another registration
system. **Then adopt swamp's rule verbatim in `skills/pudl-core`:** never author
these files from scratch; always scaffold first and edit the returned path.

### Group B — ergonomics (as drafted: "Tier 2")

#### R4 — Machine-readable introspection

**Problem.** `pudl prime` is a hand-maintained 166-line document. It will drift
from the code, and there is no mechanism that makes it not drift. Swamp's
`swamp help [<command>...]` returns the entire command tree as JSON — flags,
arguments, descriptions, subcommands — scoped to any subtree, so an agent
discovers instead of guessing.

**Proposal.**

- `pudl help --json` — walk the cobra command tree and emit it. Cobra already
  holds every field needed; this is roughly 100 lines and it permanently kills
  the drift problem for the CLI surface.
- `pudl model describe <name> --json` — arms, required inputs, declared output
  schemas, `depends_on`, checks.

*A third bullet proposing `mu plugin describe --json` was cut by review: it
already exists.* `mu plugin info <name>` (`cmd/mu/plugin_info.go:23-80`) resolves
a plugin from project config or `~/.mu/plugins/`, starts it, runs `discover`, and
honours `--json`; every plugin's `discover` returns `capabilities` and
`config_schema`. `mu plugin list --discover` does the same across all declared
plugins. §2's comparison row has been corrected accordingly — mu has capability
introspection, pudl does not have it for its own models.

`prime` stays, but shrinks to concepts and routing — the parts that genuinely
need prose — with the command surface delegated to `help --json`.

#### R5 — Persist the run report (REDUCED)

**Problem.** When a swamp run fails the agent has structured diagnostics waiting
(`swamp report get @swamp/method-summary --model X --json`); the loosh repo's
CLAUDE.md has a standing rule to check them before retrying. A pudl run report is
not retrievable after the run.

**What already exists** — the earlier draft missed all of this:

- `cmd/run_report.go:12` defines `RunReport{RunID, Model, Mode, Populate, Drift,
  Checks, Converge, OK}` with full `--json` rendering. Already structured,
  already agent-shaped.
- `pudl status` persists the per-model verdict to the catalog.
- `RunID` is already stamped on every ingested catalog entry
  (`internal/mubridge/ingest.go:419-441`).

The gap is exactly one thing: `RunReport` is rendered and discarded.

**Proposal (reduced).** Persist the existing `RunReport` as a catalog artifact
keyed by `run_id`; add `pudl run report [<run-id>] --json` to read it back.
Roughly 50 lines, zero new vocabulary, and it satisfies the problem statement in
full.

**What was cut, and why.** The earlier draft proposed a
`run_phase(model, run_id, phase, status, detail)` relation, arguing that facts
are Datalog-queryable and therefore strictly better than swamp's blobs. Two
objections landed:

- It is a benefit claimed in advance of a user. Nobody has written the "failed
  converge more than twice this week" rule, and the reduced version does not
  prevent adding the relation later if someone does.
- Its defence (§8 A3) compared it to `model_depends_on`, which is *reconciled*
  per model (`cmd/run_depends.go:103`) and therefore bounded by the dependency
  graph. `run_phase` is keyed on `run_id` and grows monotonically with every
  run — and with every converge iteration, up to `--max-iters`, defaulting to 5.
  That needs a retention policy the document never budgeted for.

Introduce the relation when a rule actually needs it, with a GC story attached.

#### R6 — Value wiring between models (DEFERRED)

**Problem, which is real.** `internal/systemmodel/schema.cue:27-28` parks
Terraform-style `${vpc.id}` interpolation: `depends_on` "expresses
ordering/impact, not value interpolation." Meanwhile
`data.latest("<model>", "<data>").attributes.<field>` is the single most-used
construct in swamp's documentation. Ordering without value flow means models run
in sequence but do not compose.

**The proposed syntax does not work.** The earlier draft proposed:

```cue
desired: [{ subnet_id: catalog.latest("vpc-lookup", "vpc").attributes.subnet_id }]
```

CUE has no user-defined functions. `decodeDesired`
(`internal/systemmodel/systemmodel.go:163-189`) reads `desired` off a
fully-evaluated `cue.Value` via `LookupPath` + `List()`. There are only two ways
to make that expression resolve: pre-inject `catalog` as concrete data before
evaluation (eager materialization, not a lazy reference), or string-template
preprocessing before CUE sees the file — which is precisely the thing
`schema.cue` parked. §8 A4's defence that this is "a render-time catalog read,
not a dependency-ordered value graph" does not survive at the implementation
level; both routes are the parked feature in different syntax.

**If this is taken up later,** the bounded version is `@tag()` injection of
named scalars resolved from a catalog query at load time — CUE's own mechanism,
declared inputs only, no new expression language. That is the 20% worth
considering. Not now.

**Deferred**, which is where A4 already half-conceded it belonged.

### Group C — differentiate (as drafted: "Tier 3")

#### R7 — ~~Distribution~~ (CUT)

**Cut by review — both mechanisms it proposed to build already exist.**

- `mu plugin` already has `push` (publish to the configured OCI cache), `list
  --remote` (enumerate plugins in remote OCI caches, `cmd/mu/plugin_list_remote.go`),
  `list --cached`, `list --discover`, `info`, `status`, `add`, `test`
  (`cmd/mu/plugin.go:35-53`). That is a publish path, an index, and a pull path —
  over OCI, which is better than the ad-hoc git index file the draft proposed.
- `pudl module add cue.dev/x/k8s.io@v0` already consumes from CUE's own module
  registry (`cmd/module.go`, via `cue mod tidy`). Overloading `pudl module` with
  a second, incompatible meaning — "bundle of schemas + rules + models +
  populators from a git URL" — would be a regression in a noun that currently
  means exactly one thing.

**What replaces it.** A documentation task, not a recommendation: write down how
`mu plugin push` / `list --remote` and `pudl module add` compose into a
distribution story. If a curated index is still wanted after that is written
down, it is a README in a git repo, not a feature.

#### R8 — Approvals on converge

**Problem.** `pudl run --converge` has `--dry-run` and `--max-iters`, but the
loop is all-or-nothing once started. Swamp has `workflow approve` / `reject` /
`resume` and a pending-approvals list.

**Implemented boundary.** `pudl run --converge --require-approval` persists a
pending request containing the model and converge options, emits a durable
pending report, and halts before any apply. `pudl run resume <run-id>` (also
`approve`) re-enters the normal converge path, which re-observes before any
apply; `pudl run reject <run-id>` is terminal. This slice deliberately does not
persist a separate plan/digest/requester artifact: the existing `--dry-run`
report remains the plan-inspection path, and a later design can add plan
identity if an operator needs approval to bind to an exact action set.

**Why it matters.** It is the difference between "the agent may observe prod"
and "the agent may change prod." That is a trust boundary users will want before
they enable convergence at all.

#### R9 — Route `SKILL.md` at `pudl guide` (REDUCED)

**Problem.** `skills/pudl-core/SKILL.md` is one file covering catalog, schema,
facts, Datalog, models, run/converge, memory loop, and the mu bridge. One file is
either too shallow or too expensive to load.

**The draft's answer was wrong.** It proposed creating
`skills/pudl-core/references/{catalog,schema,facts,query,model,converge,memory,troubleshooting}/guide.md`
and extending `internal/skills/gen` (which globs `skills/*/SKILL.md`) to walk
subdirectories. But `cmd/guide.go` is **642 lines and already ships those
topics** — `overview`, `import`, `schemas`, `facts`, `datalog`, `models`, `mu`,
`agents`. The draft would have created a second hand-maintained copy of the same
eight documents, which is precisely the drift problem R4 objects to two pages
earlier.

The draft also miscounted the existing surface. There are three hand-maintained
agent-facing documents, not one: `prime` (166 lines), `cmd/guide.go` (642 lines,
8 topics), and `SKILL.md` (146 lines).

**Proposal (reduced).** Make `SKILL.md` a routing table whose entries are
`pudl guide <topic>` invocations. Zero new files, zero generator work, and the
guides cannot drift from the binary because they ship inside it. Where a topic is
genuinely missing — troubleshooting, the memory loop — add it to `cmd/guide.go`,
which is the one place that already has the right lifecycle.

`cmd/guide.go` at 642 lines is over the repo's 300-line preference and should be
split by topic when it is next touched. That is a file-organization item, not an
architecture one.

#### R10 — A bypass hook

**Problem.** `swamp audit` records when the agent used raw `kubectl`/`aws`
instead of a swamp model. It is self-reinforcing adoption instrumentation with a
CLI surface (`swamp audit`) and an opt-in via `swamp repo init`.

**Proposal.** A PreToolUse hook that notices `kubectl get -o json`,
`aws ... --output json`, `terraform show -json` and suggests the corresponding
`pudl run`. pudl already has hook infrastructure (`pudl hooks print|install`,
harness-targeted, idempotent, backing up settings) — this is a third hook in an
existing mechanism.

**Constraint.** Suggest, do not block. A hook that fails an agent's tool call
because it preferred `kubectl` will be uninstalled within the hour.

## 6. Non-goals

Explicitly not copying:

- **`swamp serve`, grants, tokens, OAuth.** That is swamp going enterprise. It
  requires a server, an auth model, and an authorization language, and it buys
  pudl nothing its users have asked for.
- **A worker pool with enrollment tokens and a daemon.** mu's `remote-exec`
  plugin already covers the real remote need, hermetically, per target.
- **Team datastore sync.** pudl is documented as single-writer ("anything
  needing concurrent writes from multiple processes" is on the do-not-use list).
  Changing that is a different project.
- **A TypeScript extension runtime.** Shipping an embedded Deno and inlining npm
  packages into bundles would trade away the single-static-binary property for
  a authoring language pudl's users have not asked for. mu plugins are already
  the extension point.
- **Per-instance locking as the concurrency model.** mu's graph already
  parallelizes correctly; adopting swamp's lock would import their fan-out
  workaround along with it.

## 7. Sequencing

*The draft's ordering was a cycle: R1 step 3 depended on R2, §7 put R2 after R1,
and R3 gated useful adoption of both. Rewritten as a flat list, unblocked first.*

| # | Item | Blocked by | Size |
|---|---|---|---|
| 1 | R1.1 — fix `resourceTypeToSchema` (multi-dot, acronym) + existence check, reconciled with `reclassify` | nothing | ~1 day |
| 2 | R1.2 — add `#Instance` to `pudl/aws` | nothing | hours |
| 3 | Fix `plugins/k8s/GUIDE.md:54` (documents `_schema` tagging the plugin does not do) | nothing | minutes |
| 4 | R4 — `pudl help --json`, `pudl model describe --json` | nothing | small |
| 5 | R9 — `SKILL.md` routes at `pudl guide`; add missing guide topics | nothing | small |
| 6 | R3 — scaffolding + `pudl run --populate plugin:…` ad-hoc flag | nothing | medium |
| 7 | R5 — persist `RunReport`, `pudl run report --json` | nothing | ~50 lines |
| 8 | R10 — bypass-suggestion hook | 6 (needs something to suggest) | small |
| 9 | R8 — approvals on converge | nothing | medium |
| 10 | R1.3 — k8s inventory observe path in mu | nothing, but it is real mu work | large |

Items 1–3 are defect fixes and should land regardless of whether the rest of
this document is adopted. R2, R6 and R7 do not appear: R2 and R7 are cut, R6 is
deferred.

## 8. Adversarial review

A hostile review was run against the first draft with an explicit
anti-complexity mandate: hunt speculative generality, frameworks where a
function would do, and abstractions built before there are two real users. It
returned 14 findings. Three recommendations were cut, three reduced, and six
factual claims corrected. The draft's own seven-item self-review (A1–A7 below)
was largely superseded; it is kept because two of its rebuttals were shown to be
self-serving, and that is worth recording.

### 8.1 Findings that changed the document

| # | Verdict | Effect |
|---|---|---|
| 1 | R2's premise false — `#SystemModel` requires only `name` + `populate`, so `#Source` duplicates a legal shape | **R2 cut**; §3.2 rewritten; ad-hoc flag moved to R3 |
| 2 | `k8s` observe is differential-only and emits no `_schema`; `terraform` observe returns a text blob | **R1 rescoped**; k8s inventory split out as real mu work |
| 3 | `pudl/linux` exists with exactly the six types `host` emits | **R1 corrected** — the one working pair was listed as broken |
| 4 | `cmd/guide.go` is 642 lines and already ships the proposed topic tree | **R9 reduced** to routing at `pudl guide` |
| 5 | `mu plugin push` / `list --remote` / `info` and `pudl module add` already exist | **R7 cut** |
| 6 | CUE has no user-defined functions; `catalog.latest(...)` is not expressible | **R6 deferred**; `@tag()` named as the bounded alternative |
| 7 | `cmd/run_report.go` already defines a structured `RunReport`; only persistence is missing | **R5 reduced** to ~50 lines |
| 9 | `mu plugin info --json` already does capability introspection | R4 bullet deleted; §2 row corrected |
| 11 | `cmd/reclassify.go` + `ListUnresolvedItemSchemas` already handle unresolved refs on a parallel path | R1.1 now required to reconcile rather than duplicate |
| 12 | §7's "R1 → R2 → R3 arc" was a cycle | §7 rewritten as a flat, unblocked-first table |
| 14 | No mu plugin declares `apply`; `ingestObserveRecord` not `ingestRecord`; `#PluginDef` has four forms; `pudl export` exists | corrected in place |

Two findings were accepted without changing a recommendation, and are recorded
as standing caveats:

- **Finding 8 — A3's growth rebuttal was wrong.** It compared `run_phase` to
  `model_depends_on`, which is *reconciled* per model (`cmd/run_depends.go:103`)
  and bounded by the dependency graph. `run_phase` is keyed on `run_id` and
  grows monotonically. The objection was about growth over time; the rebuttal
  answered per-run cardinality. This is now recorded inside R5 as a reason the
  relation needs a retention policy before it is introduced.
- **Finding 13 — nothing here is validated against a user.** The entire evidence
  base is one competitor's CLI, one competitor deployment, and that competitor's
  skill tree. No pudl user request is cited anywhere. The one piece of
  user-facing evidence (§9) argues for R3 and R4 and for nothing else. **This is
  the document's largest standing weakness and it is not fixable from inside the
  document.** Treat every recommendation below priority 4 in §7 as unvalidated
  until someone asks for it.

### 8.2 The draft's original self-review

Kept for the record. A2 and A5 did not survive.

**A1 — "R1 is a treadmill."** *Partly accepted, and the scope narrowed further
after review.* R1 is now two defect fixes plus one explicitly-costed mu item.

**A2 — "R2 adds a primitive, and CLAUDE.md says only do what is asked."**
*This was the right objection and the mitigation was wrong.* The mitigation
offered was "`#SystemModel.populate` is re-expressed in terms of `#Source`" —
which finding 10 showed to be impossible, since `#Source` could express neither
`#PluginObserve` (missing `differential`) nor `#EweTarget` (missing six fields).
The escape hatch A2 named — "reduce to the ad-hoc path with no new declared
type" — is what actually shipped, as R3's `--populate` flag.

**A3 — "R5's phase facts will explode the fact store."** *Superseded by finding
8; the rebuttal answered the wrong question.*

**A4 — "R6 reintroduces the parked interpolation."** *Upheld, and stronger than
the draft conceded.* Finding 6 showed the distinction A4 rested on does not
exist at the implementation level.

**A5 — "R4 duplicates `prime`."** *The rejection restated the proposal instead
of answering, and miscounted.* There were three hand-maintained agent-facing
documents, not one. R9's reduction addresses the real version of this.

**A6 — "The comparison flatters pudl."** *Upheld and reinforced.* Finding 2 made
the kubernetes gap larger than the draft claimed, not smaller.

**A7 — "R10 will annoy people."** *Unchanged.* Suggest, never block.

## 9. Appendix: a defect found while researching

`~/dev/loosh/dev/AGENTS.md`, in the
`union:personal:tools/pudl/when-to-schematize` block, instructs agents to "pull
once unschematized: `pudl pull <source>`" and then `pudl schema export <name>`.

Neither command does what the block describes:

- `pudl pull [scope]` retrieves **facts** from the bitemporal store, filtered by
  scope/kind/source/relation. It performs no ingestion. There is no
  "pull a source" command; the ingestion commands are `pudl import` and
  `pudl run`.
- `pudl schema export` does not exist. Top-level **`pudl export`** does ("export
  data lake entries to various formats"), and is almost certainly what was
  meant — so this half is a one-word rename, not a missing feature.

Any agent following that block will fail on its first command. The fix belongs
at the union source, not in the generated `AGENTS.md`.

## 10. Follow-up grilling: decisions and boundary conditions

This section turns the review into an implementation contract. Each question
was checked against the current PUDL/mu code and the live swamp deployment. The
recommended answer is the one used by the execution plan below.

### G1 — Is `#SystemModel` the wrong unit of automation?

**No.** The schema requires only `name` and `populate`; a minimal runnable
model is a valid four-line declaration. The problem is the authoring ceremony,
not the unit's conceptual size.

**Consequence:** do not add `#Source`, a second model noun, or a parallel
runtime. Add scaffolding and an in-memory populate entry point around the
existing `#SystemModel` and ACUTE coordinator.

### G2 — Does “standard observers” mean adding PUDL schemas for every mu plugin?

**No.** The observer contract is mu plugin discovery/observe plus a resolving
PUDL schema. `pudl/linux` already matches the host observer. AWS needs routing
repair and `#Instance`; Kubernetes needs a real inventory mode in mu before
PUDL can classify its records; Terraform observe emits a plan text blob and is
not an inventory source.

**Consequence:** R1 is three separately verifiable slices:

1. repair PUDL's `_schema` routing and unresolved-reference fallback;
2. complete the already-working host/AWS pair;
3. implement and test Kubernetes inventory observation in mu.

### G3 — Should PUDL acquire a workflow engine?

**No.** mu already owns action-level DAG construction, parallel execution,
hermeticity, and CAS. PUDL owns the semantic ACUTE lifecycle: populate, drift,
checks, converge policy, and verdict authority. Swamp's workflow-level
conditions and approvals are useful affordances, but duplicating its DAG engine
in PUDL would create two schedulers with different semantics.

**Consequence:** implement approval/resume at the PUDL converge boundary, while
leaving action scheduling and fan-out in mu.

### G4 — Is a new run-phase fact relation the right diagnostic substrate?

**Not yet.** `RunReport` already contains the structured phase results and is
currently rendered then discarded. A durable report artifact keyed by `run_id`
is the smallest complete fix. A Datalog phase relation is deferred until an
actual query needs it and a retention policy exists.

**Consequence:** R5 persists the existing report shape; it does not introduce
new run-phase vocabulary or unbounded fact growth.

### G5 — Does machine-readable discovery require replacing `prime` or `guide`?

**No.** `prime` remains the conceptual orientation, and `pudl guide` remains
the human/agent topic reference. The command tree is generated as JSON, while
the embedded guides remain the prose authority. `pudl model show --json`
already exists and should be enriched or paired with a narrowly scoped
`describe` surface rather than another hand-maintained document tree.

### G6 — Should cross-model value wiring enter this implementation pass?

**No.** `depends_on` currently means ordering and impact, not interpolation.
CUE has no user-defined `catalog.latest(...)` function, and eager tag injection
would be a new value-resolution mechanism. This is a real gap, but the current
plan must first make individual models useful, discoverable, inspectable, and
recoverable.

**Consequence:** the plan ends at a stocktake, with explicit evidence for what
the pre-value system can do. No value-reference syntax, evaluator, or lazy
catalog lookup is implemented here.

### G7 — What counts as done for parity?

Parity means the first useful path is short and safe:

```text
discover capability
  -> scaffold or invoke an ad-hoc model
  -> observe typed state
  -> inspect drift/checks
  -> optionally approve and converge
  -> retrieve the durable report and snapshot
```

It does not mean copying swamp's Deno runtime, registry, server, team
datastore, grants, or worker pool.

## 11. Executable delivery plan

Each slice has one owner boundary, one user-visible outcome, and an acceptance
gate. A slice is not complete when its unit tests pass; its contract test,
documentation, and cross-repo behavior must agree.

### Slice 0 — freeze the contract and baseline

**Owners:** PUDL + mu. **Output:** this document and a recorded baseline.

- Keep R6 out of all implementation diffs.
- Record current CLI help, toolchain versions, dirty-tree boundaries, and the
  existing PUDL/mu integration tests.
- Treat PUDL's two new design documents as user work and mu's existing dirty
  changes as unrelated unless a slice explicitly touches them.

**Gate:** `mise exec -- go test ./...`, `mise exec -- go vet ./...`,
`mise exec -- go build ./...`, PUDL skill-generation check, and the relevant mu
test/build gate are green before the first mutation.

### Slice 1 — repair the observe-ingest contract

**Owner:** PUDL. **Outcome:** every declared `_schema` either resolves to a
real schema or falls back to inference; no new observe row carries a dangling
schema reference.

- Split resource types at the final component, so `aws.ec2.vpc` maps to
  `pudl/aws.#VPC` rather than `pudl/aws.#Ec2.vpc`.
- Preserve acronym names such as `VPC` and `NATGateway` through an explicit
  mapping/lookup strategy rather than guessing from capitalization alone.
- Reuse the existing unresolved-reference/reclassification machinery where it
  applies; do not create a second unresolved state model.
- Add existence checks against the loaded schema graph before accepting a
  plugin-declared reference.
- Add `pudl/aws.#Instance` matching the AWS observer output.

**Gate:** routing unit tests cover multi-dot names, acronyms, underscores,
malformed tags, and unknown tags; an AWS observe fixture lands only resolving
schema refs; `pudl reclassify` behavior remains intact.

### Slice 2 — make Kubernetes inventory a real observer mode

**Owner:** mu. **Outcome:** a Kubernetes model can inventory typed resources
without authored desired manifests.

- Add an explicit inventory configuration/observe path to the k8s plugin; do
  not infer inventory from an empty desired source list.
- Define the resource selection contract: kinds, namespace/all namespaces,
  context, kubeconfig, and whether discovery is bounded or cluster-wide.
- Emit stable `_schema` resource types and records compatible with PUDL's
  ingest contract, while retaining the existing differential response shape
  for convergence models.
- Update the incorrect k8s guide claim about `_schema` output.
- Test the plugin with a fake `kubectl` boundary or deterministic command
  fixture; separately test the PUDL ingest path with representative records.

**Gate:** inventory with no desired manifests returns queryable typed records;
differential observe remains unchanged; existing plan/apply behavior remains
unchanged; the PUDL-to-mu guide documents both modes.

### Slice 3 — remove first-use authoring ceremony

**Owner:** PUDL. **Outcome:** an agent can discover a plugin and observe it in
one invocation, while persistent use is scaffold-first.

- Add `pudl model new <name> --populate plugin:<name>` with `--json`, returning
  the written path and a valid model skeleton.
- Add `pudl run --populate plugin:<name> --input key=value` as an ad-hoc,
  observe-only path that writes no model file and reuses the normal run/report
  lifecycle.
- Validate plugin capability/configuration before the run when discovery is
  available; surface the plugin's config schema in errors and descriptions.
- Add `pudl rule new <name>` and `pudl model populator new <model>` only if
  their generated output is exercised by a real path in this slice; otherwise
  leave them as separately tracked follow-up work.
- Change `skills/pudl-core` to say scaffold first and edit the returned path.

**Gate:** a temp workspace can scaffold, inspect the generated path, and run an
observe-only ad-hoc invocation without hand-authoring registration CUE; ad-hoc
execution leaves no definition file and cannot mutate the target. Plugin
capability discovery is advisory when available: a discovered plugin without
`observe` is rejected, while an unavailable discovery service is reported and
the cached plugin path remains usable.

### Slice 4 — make the surface discoverable by machines

**Owner:** PUDL. **Outcome:** agents can discover the CLI and a model's
required/runtime contract without parsing prose.

- Add `pudl help --json` from the Cobra command tree, including flags,
  positional arguments, descriptions, and child commands.
- Add or enrich `pudl model describe <name> --json` with populate/converge arms,
  plugin capabilities/config schema, desired/check counts, dependencies, and
  output/schema expectations.
- Keep `prime` conceptual and `guide` topic-oriented; remove duplicated
  command inventories when the generated surface is available.

**Gate:** a snapshot test of the JSON schema covers representative nested
  commands and flags; generated help includes every registered command; model
  description reflects the actual CUE instance.

### Slice 5 — make run outcomes durable and recoverable

**Owner:** PUDL. **Outcome:** a failed or successful run can be diagnosed after
the process exits.

- Persist the existing `RunReport` as a catalog artifact keyed by `run_id`.
- Add `pudl run report [<run-id>] [--json]`, defaulting to the most recent
  persisted report globally when no ID is supplied; PUDL has no selected-model
  report argument yet.
- Preserve the distinction between `clean`, `drifted`, `failed`, and
  `unknown`/needs-verification.
- Link the report to its observation snapshot and manifest artifacts without
  duplicating phase facts.

**Gate:** a subprocess-level or integration test kills/fails a run after
populate, converge, and receipt-persistence boundaries; the report remains
retrievable and names the correct run/snapshot/artifact IDs.

### Slice 6 — add an explicit converge approval boundary

**Owner:** PUDL. **Outcome:** an operator can inspect a planned mutation,
approve/reject it, and resume without silently rerunning an unbounded apply.

- Add `--require-approval` to converge.
- Persist the request/options and mark the run `pending-approval` without
  applying.
- Add `pudl run approve <run-id>` (an alias), `pudl run reject <run-id>`, and
  `pudl run resume <run-id>` with terminal-state checks.
- Resume through the normal run path so the world is re-observed before any
  apply; never resume by blindly replaying a stale receipt.
- Make pending/approved/rejected state visible in the durable report. Exact
  plan identity and requester attribution remain a follow-up, not a hidden
  promise of this slice.

**Gate:** no approval means no side effect; reject is terminal; approve/resume
re-enters the bounded converge loop; already-terminal requests fail safely.

### Slice 7 — route agents toward the product path

**Owners:** PUDL + mu. **Outcome:** the documented agent workflow reinforces
PUDL/mu usage without blocking legitimate escape hatches.

- Add missing `pudl guide` topics for troubleshooting and the memory loop, and
  make `SKILL.md` a compact router into the embedded guides.
- Document how `mu plugin push`/remote discovery and `pudl module add` compose;
  do not build a second registry.
- Add an opt-in PreToolUse suggestion hook for raw `kubectl`, `aws`, and
  `terraform show -json` calls when a corresponding PUDL/mu path exists.
- Suggestions must be advisory and must never block or fail the underlying
  tool call.

**Gate:** installed hooks are idempotent, backed up, harness-targeted, and
  produce no suggestion when no matching model/plugin exists; docs agree with
  current command behavior.

### Slice 8 — stocktake gate; stop before value wiring

**Owners:** PUDL + mu. **Output:** a current-state report, not new R6 code.

Verify the complete first-use path against real or deterministic fixtures:

1. discover a capability;
2. scaffold or invoke ad hoc;
3. observe inventory;
4. classify records with resolving schemas;
5. inspect drift/checks and snapshots;
6. exercise the dry-run/approval boundaries, then converge and re-observe
   through the bounded ACUTE loop;
7. retrieve the durable report after success and failure.

Record remaining gaps, test evidence, dirty-tree state, and the exact proposed
call site that would justify cross-model value wiring. Only after this gate may
R6 be designed.

## 12. Verification matrix

| Requirement | Strong evidence |
|---|---|
| No dangling observer schema refs | fixture ingest + catalog query validating every ref |
| AWS/host typed inventory | mu observer output + PUDL catalog rows + schema validation |
| Kubernetes inventory | fake-cluster/plugin fixture with no desired manifests |
| Scaffold-first UX | temp workspace command transcript + generated file + validation |
| Ad-hoc UX | command transcript proving no file and no mutation |
| Machine discovery | generated help/model JSON snapshot tests |
| Durable diagnostics | failed subprocess/integration run, then report retrieval |
| Approval safety | side-effect counter around reject/approve/resume/crash cases |
| Agent routing | generated skill check, hook install/idempotence tests, docs sweep |
| Stocktake boundary | final report explicitly showing no R6 implementation |

The final claim is not “tests pass.” It is that every row above has direct
evidence from the shipped path, and that the stocktake can name what remains.

## 12.1 Implementation evidence (2026-07-27)

The pre-value-wiring slices are implemented in the current worktree:

- PUDL: `mise exec -- go test ./...`, `mise exec -- go vet ./...`, and
  `mise exec -- go build ./...` pass.
- mu: `mise exec -- go test ./...` and `mise exec -- go build ./...` pass.
  `bb plugins/k8s/plugin.bb </dev/null` passes, and a fake-`kubectl` smoke
  returns inventory records tagged `_schema: "k8s.resource"`.
- Observe routing has multi-dot/acronym tests, loaded-schema existence checks,
  inference fallback coverage, AWS EC2 instance bootstrap schema coverage, and
  catalog ingest tests for typed AWS and Kubernetes inventory. A missing schema
  graph now falls back to the generic observe schema rather than accepting an
  unverified naming-convention reference.
- An isolated CLI smoke built a model scaffold with `--json`; a separate
  isolated ad-hoc run used a fake mu/plugin boundary, produced one inventory
  record, persisted a report retrievable with `pudl run report --json`, and
  left no ad-hoc model definition behind.
- The approval store has round-trip and terminal-state tests, and resumed runs
  retain `approved` in the durable report. Command-level tests cover the help
  tree, model runtime-contract description, scaffold input, report persistence,
  and failure rendering; the surface exposes reports, resume/approve, reject,
  scaffolds, guides, generated skill routing, and advisory hooks.

Known boundary conditions are intentional: live cloud/cluster credentials were
not exercised, the isolated scaffold validation could not fetch its optional
CUE registry dependencies in the sandbox, and approval is request-level rather
than exact-plan-level. No cross-model value-wiring implementation was added.

## 13. Deferred: cross-model value wiring

The next design must start from one concrete value-flow request, for example
“model B needs the subnet ID produced by model A,” and answer:

- what artifact is the authoritative producer output;
- when it is resolved relative to model loading and execution;
- how freshness, failure, and absence are represented;
- whether the value is a scalar, a record, or a query result;
- how CUE validation sees unresolved values;
- how the dependency graph and run report expose the relationship.

No `${...}` syntax, lazy catalog function, or general interpolation mechanism is
authorized by this roadmap before that design pass.
