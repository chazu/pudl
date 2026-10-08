# Design: simplifying the cataloging workflow (GCP findings follow-up)

**Date:** 2026-10-08
**Implements:** the verified proposals derived from
`docs/gcp-cataloging-workflow-findings.md`
**Status:** implemented 2026-10-08 (all four phases; deviations in §11). User
docs: `docs/projection.md`, `docs/cli/notes/{import,run,list,export}.md`.

## 1. Background

An agent cataloged GCP resources with pudl using this loop:
`gcloud … --format=json` → `jq` (inject `project`) → hand-written CUE schema →
`pudl import --schema` → `jq` over `pudl show --raw` to answer questions. Its
findings were checked against the source. Corrections to the findings:

- `--path` already accepts a directory or glob. An *unquoted* glob is
  shell-expanded and every file after the first is silently ignored, because
  `importCmd` ignores positional args.
- A model can run an arbitrary command today, but only via mu: the command
  must speak mu's discover/observe protocol and a mu root must exist.
- NDJSON items are inferred one by one.
- `pudl export --id X --format json` already prints exact payloads, and
  `pudl list --json` carries `total_matched`. Both are documented rather than
  duplicated.

Gaps that the findings missed:

- **Quoted dotted identity keys fail silently.** On error the importer nils the
  identity and falls back to the content hash, so the Cloud Run resources never
  form version chains, and nothing reports it.
- **Re-importing with a corrected `--schema` is a no-op.** Item dedup by
  content hash is global, and identical files are skipped whole.
- **`list --json` paginates at 20.**

## 2. Goals and non-goals

Goal: remove each manual step of the loop above and each silent failure in it.
The bar is reached when a user can go from `gcloud` output to a trustworthy
`#Check` verdict with a schema, a rule and a model, and no `jq`.

Non-goals (each was considered and deferred):

- **No broadening of schema inference.** Trying every schema when only the
  catchall scores would let open-struct user schemas claim foreign records.
  Routing is explicit (`--schema`, or the command arm's `schema:`).
- **No deletion detection for projected facts.** A resource missing from a
  later observation keeps its facts current.
- **No datalog negation and no string `!=`.** Absence is made expressible
  through projection (`default`, `exists`; §5.2) instead.
- **No `acute.RecordIdentity` change.** It is top-level-key only, which is
  inventory drift. That is follow-up work.
- **No sealed inputs on the command arm, and no hash mode for sensitive fields**
  (an unsalted hash is brute-forceable).
- **No retroactive scrub of payloads stored before a field was declared
  sensitive.** A future `pudl doctor` check is noted in §6.

## 3. Shared primitives

### 3.1 Field paths (`internal/fieldpath`, pure)

```
path    := segment ("." segment)*
segment := (ident | quoted) ("[*]")*
ident   := any run of characters other than '.'
quoted  := '"' JSON string body '"'          // may contain dots
```

Examples: `metadata.labels."cloud.googleapis.com/location"`, `sourceRanges[*]`,
`allowed[*].IPProtocol`.

API: `Parse`, `MustParse`, `FromKeys(...string)`, `HasWildcard`, `Lookup`,
`LookupAll`, `Set`, `Replace`.

Identity extraction uses it, and wildcards are rejected there. The
top-level-array → first element behavior of `ExtractFieldValues` is kept.

**Compatibility (honest version):**

- Every identity path that resolved before resolves to the same value, so its
  `resource_id` is unchanged.
- Paths that *failed* before may now resolve: quoted segments, and segments
  ending in `[*]`, which were never meaningful keys. New imports of those
  records get identity-based rids, so they start new chains. Run
  `pudl identity migrate --recompute` (followed by the projection sync, §5.5)
  to rebuild old ones.
- Empty segments (`a..b`) become a parse error. That is reported, not silent
  (§4.2).
- `inference/heuristics.go` keeps its top-level-key scoring. It is a score, not
  identity.

### 3.2 CLI values (`cmd` shared parser)

`parseCLIValue(raw string) any` is used by `import --set`, `model new --input`
and `pudl query` constraints:

| Raw value | Result |
|---|---|
| valid JSON number | `json.Number` (exact; for query, via `database.QueryNumber`) |
| `true` / `false` | bool |
| `null` | nil |
| JSON-quoted string `'"123"'` | string (forces string) |
| JSON object or array | decoded exactly |
| anything else | raw string |

The query change makes `pudl query rel disabled=false` match projected booleans
(`pudl_query_value` maps JSON booleans to 1/0, and SQLite binds Go bools the
same way).

## 4. Import

### 4.1 Paths

`pudl import a.json b/*.json dir/` is accepted (`Args: cobra.ArbitraryArgs`).
Positional args and `--path` are merged, then each is resolved by
`resolveFilePaths`, deduplicated and kept in order. Positional paths are checked
**before** the stdin probe, so an agent with piped stdin still imports the named
files.

### 4.2 Identity resolution is reported

`ImportResult` gains `identity_unresolved` (a count) and `identity_error` (the
first error, naming the schema). The human summary warns that these records are
identified by content hash and will not form version chains. The fallback is
unchanged; data is never rejected for this.

### 4.3 `--set path=value`

`--set` is repeatable and **overwrites**. The path is a field path (§3.1, no
wildcard) and the value goes through `parseCLIValue`. It applies to every record
of an NDJSON or JSON-array collection and to a single-object JSON document. Any
other shape is an error.

`--set` is for *data*, such as the `project` that gcloud omits. It is not a
routing mechanism; use `--schema` for routing. With one `--set` per invocation,
per-project files are imported one invocation each, or the command arm's `runs`
(§7) does the fan-out.

### 4.4 The transform pre-pass

The pre-pass runs when `--set` is given or the redaction gate (§6.1) is open.

1. **Decode.** Records are decoded with `idgen.DecodeJSONExact`, streaming:
   collections through `ingestprep` under the same limits, a document as one
   value.
2. **Apply `--set`.**
3. **Assign and redact (only when the redaction gate is open).** The schema is
   assigned (inference or `--schema` chain). The assignment's schema name,
   confidence, reason and source are **spooled to disk**, one line per record,
   as an NDJSON file. Records then have sensitive fields redacted (§6). Explain
   traces are kept only for the first 32 records, which is already the explain
   cap.
4. **Write.** The transformed records go to a temp file under the import temp
   dir, **keeping the source's extension and collection format** (NDJSON →
   NDJSON, JSON array → JSON array, document → JSON). `OriginPath` is set only
   if it is empty, so envelope and decompressed origins survive.
5. **Hand off.** If no record changed (no `--set` and zero redactions), the
   original file is used unchanged, so unrelated imports keep their hashes and
   still dedup. Otherwise the normal pipeline runs on the transformed file.
   `prepareItem` and `importDocument` read the spooled assignment in lockstep
   instead of re-inferring, because redacted values may not satisfy the
   schema's types. The main pass's `DecodedBytes` budget becomes
   `max(limit, transformed size)`.

Content-hash invariants hold: collections and documents hash their stored bytes,
and items hash canonical decoded JSON. `VerifyPayloads` and dedup need no
special cases.

### 4.5 Re-import with `--schema` reassigns

When `--schema` is given and an item (or document) deduplicates against an
existing entry under a different schema, the importer validates the record
against `--schema`. If the intended schema validates, it reassigns that entry
inside the import's catalog transaction:

- the schema
- `resource_id` and `identity_json`, recomputed
- the version, re-sequenced to `latest(new rid) + 1` when the rid changed
- the `.meta` schema info, rewritten with the journal

When the whole file was already imported, the item pass still runs with
`--schema` set, so reassignment applies there too. `ImportResult.reassigned`
counts reassigned entries. Facts follow automatically through the projection
sync (§5.5).

### 4.6 `--dry-run`

`--dry-run` runs decode → `--set` → assign → redact → identity → projection per
record and writes nothing: no staging, no catalog rows, no facts. The DB is
opened read-only. On a database that has never been opened read-write, dedup
lookups are skipped and that is reported.

Output reuses the import result shape. `--json` gives the same array of
per-file outcomes, with `status: "dry-run"` and these extra fields:

- `schemas`: count per assigned schema
- `already_cataloged`
- `would_reassign`
- `validation_failures`, with up to 5 issues (values stripped for sensitive
  schemas)
- `redacted`
- `facts` per relation
- `identity_unresolved`
- `ambiguous`

Exit code is 0 unless an error occurs. `--explain` adds traces in either mode.

**Ambiguity** is computed only under `--dry-run`/`--explain`. After the first
match, up to two further candidates with the same score are tried, excluding
candidates that share the winner's `IdentityRoot` (base/child pairs). It is
reported as `X also matched Y`. The documented remedy is a discriminator in the
schema, such as `kind: "compute#firewall"`.

### 4.7 Exact schema filter

In `database.QueryEntriesContext`, so `list`, `export` and `reinfer` agree:

- **Value containing `#`:** after `schemaname.Normalize`, it matches
  `schema = v`, `schema LIKE '%/' || v`, or (when v starts with `#`)
  `schema LIKE '%.' || v`. All of these are escaped. So `aws.#EC2Instance`
  still finds `pudl/aws.#EC2Instance`, and `#Route` finds routes but not
  `#Router`.
- **Value without `#`:** an escaped substring match, as today.
- **`Origin` and `Format`:** escape their `LIKE` input.
- **Empty result hint:** when an anchored filter returns nothing but a
  substring match would not, `pudl list` prints a hint.

## 5. Fact projection

### 5.1 Why

`pudl query` and `#Check` cannot see imported fields. Schemas now declare the
relations their records project into. Facts are the existing bitemporal store,
so rules, checks and history work unchanged.

### 5.2 Syntax (`_pudl.facts`)

```cue
_pudl: {
	resource_type:   "gcp.firewall"
	identity_fields: ["project", "name"]     // required when facts is set
	facts: {
		gcp_firewall: args: {
			project: "project", name: "name", direction: "direction", network: "network"
			disabled: {path: "disabled", default: false}
		}
		gcp_firewall_source: args: {range: "sourceRanges[*]"}
		gcp_firewall_allow: {
			each: "allowed[*]"                          // one fact per row
			args: {proto: "IPProtocol", port: {path: "ports[*]", default: "*"}}
		}
	}
}
```

**Relation shape.** A relation is `{each?: path, args: {[name]: argspec}}`.
`argspec` is one of:

- a path string
- `{path, default?, trim_prefix?}`
- `{exists: path}`, which yields a bool

**Rules:**

- **`each`.** The path must contain a wildcard. Arg paths are then relative to
  each row, and record-level fields are reached by joining on the implicit
  `entry_id`.
- **One wildcard arg.** At most one arg per relation (relative to the row) may
  contain a wildcard, so there are no cartesian products. Pairing such as
  proto/port is preserved through `each`.
- **Absent values.** If a path is absent, the `default` is used when given;
  otherwise the arg is omitted. If a wildcard arg yields nothing and has no
  default, the relation produces no facts for that row.
- **`trim_prefix`.** Strips a string prefix, e.g.
  `"https://www.googleapis.com/compute/v1/"`, so references in URL form and in
  path form join. Short-name references, such as a GKE `subnetwork:
  "default"`, are not resolvable; this is documented.
- **Values.** Scalars keep exact `json.Number` text. Numbers are checked with
  `database.QueryNumber`, and out-of-domain values are omitted and reported, so
  one uint64 cannot poison a relation. Objects and arrays are stored as JSON.
- **Implicit args.** `entry_id` and `resource_id` are added to every fact. Join
  to the catalog with `catalog_entry(id: $E)` and `entry_id: $E`.
- **Inheritance.** The relation map is the union along `base_schema`; on a name
  collision, the nearer schema wins.

### 5.3 Validation at schema load

Projection specs are decoded strictly, in a separate pass over `_pudl.facts`,
with the error kept. A schema with an invalid spec has projection **disabled**,
and the import reports a warning per file. Data is still imported.

A spec is invalid when any of these hold:

- the relation or arg name is not a datalog identifier
  (`^[A-Za-z_][A-Za-z0-9_]*$`)
- the relation is reserved (`catalog_entry`) or an arg is named `entry_id` or
  `resource_id`
- a path does not parse
- the spec has more than one wildcard arg
- `each` has no wildcard
- `facts` is present without `identity_fields`

Rule-head collisions (a relation that is also a rule head) are warned about by
the rule lint (§5.6).

### 5.4 Currency: `projection_state`

A new table, added by a numbered migration:

```sql
CREATE TABLE projection_state (
  resource_id TEXT PRIMARY KEY,
  entry_id    TEXT NOT NULL,     -- the most recently OBSERVED entry for this resource
  schema      TEXT NOT NULL,
  fingerprint TEXT NOT NULL,     -- hash of the effective facts spec + sensitive set
  status      TEXT NOT NULL,     -- projected | skipped (identity unresolved)
  observed_at INTEGER NOT NULL
);
CREATE INDEX idx_current_facts_source ON current_facts(source);
```

Facts are written with `source = "projection:" + resource_id`. The `projection:`
prefix is reserved: `pudl facts add` and `factstore.AddFact` reject it.

**"Latest" means most recently observed, not max version.** Observe entries are
all version 1, and a reverted resource dedups to an old entry. Both cases are
handled because every observation of a resource, including a dedup hit, updates
`projection_state`.

### 5.5 Reconciliation

`CatalogTx.ReconcileProjection(source string, want []Fact, mode)`:

1. Read the current facts for `source` and key them by
   `relation + canonical(args)`.
2. Facts in `current` but not in `want` are closed. **Observation** mode uses
   `InvalidateFact` (the world changed). **Correction** mode uses `RetractFact`
   (our belief was wrong, e.g. a spec change, reassignment or deletion).
3. Facts in `want` but not in `current` are added with `valid_start = now`. If
   `AddFact` returns a fact that is already closed (an ID collision with an
   invalidated fact), it retries with `valid_start + 1`, up to 3 times. This
   makes re-wanted facts reappear (the A→B→A case).

**Hot path (observation mode).** This runs inside the same catalog transaction
as the entry write, for every item and document, **including dedup hits**, in
both the importer and observe ingest. If the effective schema has facts:

- If the identity is unresolved, upsert the state as `skipped`, reconcile the
  source to ∅, and count it.
- Else, if the state already names this entry with the current fingerprint,
  do nothing.
- Otherwise, compute the facts (from redacted data), reconcile, and upsert the
  state.

Facts are computed at prepare time and spooled with the item descriptor,
counted against `StagingBytes`.

**Sync (correction mode)** is `projection.Sync(db, registry)`. It is idempotent,
and a no-op when nothing changed:

1. **Orphans.** For each state row whose entry is gone, whose entry's
   `resource_id` differs, or whose entry's schema no longer has facts:
   reconcile the old source to ∅ and delete the row. This covers delete, prune,
   reinfer, identity migrate, import reassignment, and observe's
   `UpdateEntryIdentity`.
2. **Stale.** Rows whose fingerprint differs from their schema's current
   fingerprint are recomputed from their entry.
3. **Bootstrap.** For entries of schemas with facts whose rid has no state row,
   the most recently imported entry per rid (by `import_timestamp`, then rowid)
   is projected and recorded.

Sync runs at the end of `pudl import` (non-dry-run), before checks in
`pudl run`, and at the end of `schema reinfer`, `delete`, observe snapshot
prune and `identity migrate`. It can also be run explicitly with
`pudl facts reproject [--dry-run]`. `pudl query` is read-only: it calls
`projection.Stale` and prints a warning naming `pudl facts reproject` when a
sync is pending.

Projection redacts payloads in memory with the current sensitive set before
computing facts. A backfill from payloads stored before a field became
sensitive therefore cannot copy the field into facts or FTS.

### 5.6 Silent-miss guards

- **Zero-yield relations.** The import summary (and `--dry-run`) warn when a
  declared relation produced 0 facts across a file that had records of its
  schema.
- **Rule lint.** This runs in `pudl query` and `pudl run` checks and prints
  warnings to stderr. It warns when a rule body references a relation that is:
  - not a rule head, not declared by any `facts` block, not built in, and with
    no stored facts;
  - declared by a `facts` block but uses an arg the block does not declare;
  - a `facts` relation that is also a rule head.
- **Scope.** Projected facts carry no `run_id`. Checks over them are evaluated
  against current state, globally, not per run scope, as documented.

## 6. Sensitive fields (`_pudl.sensitive_fields`)

```cue
_pudl: sensitive_fields: ["spec.template.spec.containers[*].env[*].value"]
```

Each matched value is replaced with `"[REDACTED]"`. There is no other mode.
Fields are the union along `base_schema`.

### 6.1 Fail closed

- **Strict decode.** `sensitive_fields` is decoded strictly (a list of
  parseable paths). An error marks the schema's sensitive set **broken**. Any
  record whose intended or assigned schema is broken fails its import with an
  explanatory error.
- **Identity overlap.** A path that overlaps an identity field is a load error
  (broken), because redacting it would collapse every record onto one rid.
- **Intended route.** The redaction set for a record is the union of its
  **intended** schema's set (`--schema`, or the command arm's `schema:`) and
  its **assigned** schema's set. A validation fallback to base or catchall
  still redacts.
- **Gate.** The pre-pass gate is open when `--schema` names a schema with
  sensitive fields, or any loaded schema declares them. Inference can pick any
  schema, so the gate cannot be narrower than that.
- **Non-JSON documents.** A YAML/CSV document whose intended or assigned schema
  has sensitive fields fails the import. It is not stored unredacted.
- **Observe ingest.** `prepareObserveRecord` redacts after schema resolution and
  before hashing and identity. The snapshot file holds no records.
- **Validation errors.** Values are stripped from validation errors for schemas
  with sensitive fields, both in output and in metadata.

### 6.2 Documented limits

- **Temp files.** Decompressed, envelope and stdin temp files may hold plaintext
  while an import runs. They are removed on completion, but a SIGKILL can leave
  them behind.
- **Retroactive.** Payloads stored before a field was declared sensitive stay
  unredacted. Projection still redacts in memory (§5.5). A `pudl doctor` scrub
  is follow-up work.
- **Drift.** Redacted observed values never equal plaintext `desired` values,
  so do not declare desired state on sensitive paths.

## 7. Command populate arm and checks-only models

### 7.1 `#CommandObserve`

```cue
#CommandRun: {
	argv: [string, ...string]          // executed directly, no shell
	set?: {...}                        // flattened to leaf paths; overwrites
	dir?: string                       // relative to the model file
}
#CommandObserve: {
	runs: [#CommandRun, ...#CommandRun]
	schema?: string                    // exact --schema semantics (chain-validated)
	timeout?: string                   // per run, Go duration; default "10m"
}
populate?: #PluginObserve | #EweTarget | #CommandObserve
```

Fan-out uses CUE comprehensions, not a template language:

```cue
_projects: ["prod-a", "prod-b"]
populate: {
	schema: "pudl/gcp.#Firewall"
	runs: [for p in _projects {
		argv: ["gcloud", "compute", "firewall-rules", "list", "--project=\(p)", "--format=json"]
		set: project: p
	}]
}
```

KMS project × location is a nested comprehension.

**Runtime.** `Populate.Kind()` returns `KindCommand` when `runs` is non-empty.
`runCommandPopulate` runs each argv through `internal/proc`, with the inherited
environment, the working directory set to `dir` (default: the model dir), and a
per-run timeout. No mu and no mu root are involved. For each run:

- stdout is spooled to a temp file under `Limits.StagingBytes`
- the output is decoded as a stream of JSON values: an array contributes its
  elements and an object is one record; anything else is an error
- `set` is applied to each record

All records go to one `ObserveResult` for `//models/<name>:populate`, which is
ingested through `IngestObserve` with the new `ManualSchema` field. That gives
chain validation, intended-route redaction, identity, and hot-path projection.
The snapshot source is `SnapshotSourceCommand`, which is added to
`observationSources`.

**Failure.** If any run exits non-zero, times out or fails to decode, the
populate phase fails and nothing is ingested; this is documented. The error
carries the last 4 KiB of stderr. The model instance record persists argv and
`set`, so do not put secrets in them.

**Wiring.** `KindCommand` cases are added to:

- `template_summary` (so templates with inputs validate)
- `model validate`, which checks that `argv[0]` resolves and `runs` is
  non-empty
- `model show`
- plan render
- `run_populate`, which skips the plugin-metadata lookup for this kind

### 7.2 Checks-only models

`populate` becomes optional. A model without `populate` must not declare
`desired` or `converge`, which `model validate` and `pudl run` enforce. Its run:

1. runs the projection sync
2. evaluates `checks`
3. persists the verdict (clean, or drifted on a failing `fail` check)

This lets checks run over `pudl import`ed data without any populate mechanism.

### 7.3 Scaffold

- **`--populate command:<cmdline>`.** `pudl model new` scaffolds `runs:
  [{argv: [...]}]`, with the command line split shell-style (quotes and
  backslashes, no expansion) into argv. `pudl run --populate command:…` builds
  the same ad-hoc model. `--input` is rejected for `command:`.
- **`--populate plugin:<name>`.** It now emits a `plugins:` block resolved from
  mu's cache (`cachedPluginDefinition`), so the unedited scaffold runs. An
  uncached plugin errors with the install hint.

## 8. Implementation phases

1. **Primitives and CLI fixes:**
   - `fieldpath` and identity on it, plus identity reporting
   - the shared CLI value parser (`--input`, query)
   - positional import paths
   - the exact schema filter
   - docs for `export`, `list --json` and `total_matched`
   - the scaffold `plugins:` block
2. **Import transforms:**
   - `--set` and the pre-pass
   - sensitive fields (both ingest paths)
   - `--schema` reassignment
   - `--dry-run` with ambiguity
3. **Projection:**
   - spec decode/validate and the registry
   - `projection_state` migration and `ReconcileProjection`
   - hot paths (import, observe)
   - `Sync`, `Stale` and `pudl facts reproject`
   - the rule lint
   - Sync calls in reinfer, delete, prune, identity migrate and run
4. **Models:** the command arm, checks-only models, and the `command:` scaffold
   and ad-hoc form.

Each phase ships with tests, keeps new code in new files, and ends with
`CGO_ENABLED=0 go test ./...`. The work closes with an `implog/` entry and a
`docs/plan.md` update.

## 9. Public API summary

- **CLI:**
  - `pudl import [paths…] [--set p=v]… [--dry-run]`
  - `pudl facts reproject [--dry-run]`
  - `pudl model new --populate command:<cmdline>`
  - `pudl run --populate command:<cmdline>`
  - `pudl query` (typed bool and null constraints, staleness and lint warnings)
- **Schema:**
  - `_pudl.facts`, `_pudl.sensitive_fields`
  - field paths with quoted segments and `[*]`
  - `#CommandObserve` / `#CommandRun`
  - optional `populate`
- **Go:**
  - `internal/fieldpath`, `internal/projection` (`Spec`, `Registry`, `Compute`,
    `Sync`, `Stale`, `Lint`)
  - `CatalogTx.ReconcileProjection` and projection-state accessors
  - `EnhancedImporter.Preview`
  - `ImportOptions.{Set, DryRun}`
  - `ImportResult.{IdentityUnresolved, IdentityError, Reassigned, Redacted, Facts, FactWarnings, Ambiguous}`
  - `mubridge.ObserveIngest.ManualSchema`

## 10. Adversarial review log

Two reviews ran against the first draft: integrity/correctness and
UX/simplicity. Disposition:

**Adopted:**

- **Projection currency.**
  - The A→B→A dedup revert: hot path on dedup hits, plus `projection_state`.
  - Re-adding invalidated facts was a no-op: content-keyed reconcile with
    fresh `valid_start`.
  - Observe entries are all version 1: latest means most recently observed.
  - `is_latest` was meaningless, so it is dropped.
- **Orphaned projection sources.** Delete, prune, reinfer, identity migrate and
  observe re-identity are handled by Sync step 1.
- **Close semantics.** Invalidate is used for observations and retract for
  corrections.
- **Identity.**
  - Unresolved identity made facts accumulate: skipped and counted, and
    `identity_fields` is required for `facts`.
  - Identity paths that overlap sensitive fields are a load error.
  - The compatibility claim is restated (§3.1).
- **Projection value and name checks.**
  - The numeric contract is checked per projected value.
  - Reserved and invalid names are validated at load.
- **Silent misses.** Fingerprint-driven Sync, zero-yield warnings and the rule
  lint.
- **Query expressiveness.**
  - Absent values: `default` and `exists`.
  - Row pairing: `each`.
  - Reference formats: `trim_prefix`.
- **Redaction fails open.**
  - Intended-route redaction.
  - Strict decode.
  - Non-JSON fails closed.
  - Validation values are stripped.
- **Pre-pass.**
  - Unrelated imports were rewritten: the original file is handed off when
    unchanged.
  - Memory: assignments are spooled to disk.
  - The decoded-byte budget is adjusted.
  - The extension and `OriginPath` are kept.
- **Routing.**
  - `--set _schema` is not routing.
  - The arm gets `schema:` with `--schema` semantics.
  - `resource_type` stamping is dropped.
- **Command arm.**
  - `foreach`/templating replaced by `runs` built with comprehensions.
  - Format is auto-detected.
  - No `sh -c`.
  - `KindCommand` wiring.
  - Its own snapshot source.
  - Stdout spooled to disk.
- **Checks-only models.** `populate` is optional, so checks work on imported
  data.
- **Re-import reassignment.** Re-import with `--schema` reassigns inside the
  import transaction with version re-sequencing, replacing `reinfer
  --to/--collection`.
- **Exact schema filter.** It now anchors on `/` and `.`, so `aws.#EC2Instance`
  keeps working.
- **CLI values and paths.**
  - One CLI value parser with exact numbers and typed booleans.
  - Positional paths are checked before stdin.
- **Cuts.** `show --payload` and `list --count` (documented existing features
  instead), the `hash` sensitive mode, and `is_latest`.
- **Ambiguity.** Only under `--dry-run`/`--explain`, excluding a shared
  identity root.
- **Safety.** The `projection:` source prefix is reserved. The migration is
  numbered.

**Documented instead of fixed:**

- temp-file plaintext windows
- no retroactive scrub
- sensitive paths in `desired`
- run-scoped checks do not see projected facts
- the whole fan-out fails on one bad run
- short-name GCP references
- no string `!=`

**Kept despite a deferral suggestion:** the scaffold `plugins:` block, because
the unedited scaffold is otherwise broken.

## 11. Implementation notes (deviations from the text above)

- **§4.5:** reassignment updates the catalog row (schema, `resource_id`,
  `identity_json`, version); it does not rewrite the entry's `.meta` file,
  matching `pudl schema reinfer`. The catalog row is authoritative.
- **§4.6:** `--dry-run` opens the catalog the way every command does, so
  idempotent migrations may run; no import data, rows or facts are written.
  Under `--json` the findings sit in a nested `preview` object on each
  outcome (status `dry-run`) rather than flat fields.
- **§5.5:**
  - `Sync` runs as one catalog transaction.
  - `pudl query` keeps its existing read-write open; it only *reports* pending
    work (`projection.Stale`) and lint warnings, and never syncs.
  - Re-observation through a dedup hit also covers whole-file dedup: a
    re-imported collection walks its items, so the A→B→A revert works for
    whole files.
- **§5.6:** zero-yield warnings cover relations of schemas whose records were
  projected (identity resolved); records skipped for unresolved identity are
  reported by the identity warning instead.
- **§6.1:** as documented, redaction follows the intended and assigned
  schemas. A record that inference assigns to an unrelated schema (no
  `--schema`) gets that schema's set. `docs/projection.md` tells users to
  route sensitive data explicitly.
- **§7.3:** the `command:` scaffold includes a commented `schema:` line to fill
  in.
- **Hardening beyond the text:**
  - `ParseRelations` decodes strictly: unknown keys are errors.
  - A broken facts spec never empties existing facts; it is reported by import,
    run, query and `facts reproject`.
