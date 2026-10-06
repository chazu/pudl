# Evidence integrity and explainability: second improvement round

Implemented all eleven retained recommendations from the October 5 review.
Independent I/O, storage, and lifecycle changes were developed in isolated
worktrees and integrated with prepared ingestion and portable recovery.

## Delivered

- Exact JSON observations and Ewe output arrays; trailing input is rejected.
- Format-aware disk-spooled export, normalized collection traversal, exact JSON
  and retained YAML numeric tokens, deterministic scalar CSV, flushed errors,
  atomic output files, and explicit failing partial exports.
- Transactional deletion before reference-aware file cleanup; active approval,
  manual, report, and eligible current evidence cannot be deleted underneath a
  consumer. Single-document versions/dedup are allocated atomically; doctor
  reports historical duplicate versions without renumbering them.
- Independent snapshot retention owners, a 30-day default report evidence
  window, historical pruning tombstones, and available/pruned references when
  reading reports. Expiry timestamps use SQLite-readable canonical UTC text.
- Frozen previous/current/expected findings with literal field paths, exact
  numbers/null, and explicit no-baseline/absent/ambiguous/incompatible/pruned
  states. Historical compatibility checks follow the comparator's actual
  name/path/id fallback for resource-type tags.
- Bounded deterministic failed-check witnesses, gating/advisory scope, sealed
  reference redaction, and human findings containing drift plus failed checks.
- Context-aware SQL/recursive evaluation/transactions/locks, public QueryContext,
  query timeout, and cancellable SQLite writer contention with bounded rollback.
- Prepared collection/observation records in private files and descriptor spools
  before the write transaction; limits cover decoded bytes, individual values,
  and preparation storage. Publication/commit/abort share a narrow artifact lock.
  Orphan metadata can be replaced only after proving it unreferenced.
- Opt-in classification candidate/fallback/source/shadowing traces and persisted
  original reasons. Reused records use their original stored explanation.
- Verified portable bundles with incremental SQLite online backup preserving
  rowids, referenced file checksums, safe extraction and private migrations,
  moved storage paths, inert approvals, unknown current status, and restored
  snapshot exclusion from producer reuse. Known legacy phantom manifest metadata
  is normalized only in the private snapshot and recorded in manifest notes.
- `doctor --verify-payloads` and source bundle verification check available SHA256
  claims, canonical JSON item hashes, and referenced file presence without repair.

Migrations 21 and 22 add retention ownership and restored-snapshot eligibility.
Mu retains execution ownership, exact-set scope and plan approval remain strict,
sealed values stay in the provider path, and generic facts/Datalog remain.

## Acceptance and limits

Meaningful regressions cover large adjacent integers, buffered export errors,
invalid formats preserving destinations, failed SQL deletion preserving evidence,
concurrent versions, independent pins, immutable historical values, exact path
components, scoped/redacted witnesses, cancellation and recovery, pretransaction
preparation, aggregate budgets, and crash-orphan reimport.

Bundle acceptance restores under a different root and checks raw evidence,
metadata, versions, shared collection membership, temporal facts/transaction
sequence, report bytes, gapped snapshot ordering, stale approvals, and historical
versus newly live producer eligibility. Corruption, truncated/appended archives,
future schemas, path traversal, symlinks, missing references, cancellation,
existing destinations, and byte limits fail without publishing a workspace.

The actual Git example CLI journey covers clean/changed inventory, explicit
missing baseline, failed-check witnesses in human/JSON, immutable report replay,
evidence availability, and named report pins. The real-Mu smoke checks previous
values across live baseline/change/repeat observations in addition to strict
sealed routing, approval/resume/reject, stale plans, and concurrent sets.

The array-framing fuzz target completed 320,426 executions in 15 seconds without
a failure. An existing fact-history test sampled the wall clock after invalidation
and assumed the same second; it now asserts the timestamp against the operation's
start/end bounds.

An initial parallel race run hit the default ten-minute timeout in the existing
300-seed differential oracle. Paired 40-seed probes measured baseline 104.560s
versus context changes 106.102s, with essentially identical CPU profiles. The
default race gate now keeps all seeds and checks, uses two concurrent packages,
and allows twenty minutes per package; CI invokes that same Make target.

Large nonstreamed documents are bounded by their complete input size; an
observation's record limit applies to one target envelope. Bundles cover the
self-contained PUDL state root, not external executables/projects/providers or
global schema fallbacks. Checksums detect damage, not creator authenticity.
External Docker/Kubernetes infrastructure qualification was not run.

Final validation passed:

- `mise exec -- make test` (`go test ./...`).
- `mise exec -- go test -race -p 2 -timeout 20m ./...`; all packages passed,
  including the full oracle (`internal/datalog`, 848.686s).
- `go vet ./...` and `go build ./...` under the pinned mise toolchain.
- `make lint vulncheck`: zero lint issues and no vulnerabilities found.
- `make check-docs check-skills` and help JSON golden.
- `make test-git-walkthrough` and `make test-kick-tires` using real Mu v0.3.5.
- Focused native frame fuzzing and the new end-to-end/regression tests above.

The original implementation was staged as one combined commit and fast-forwarded
to local main as `b3a9b34`; it had not yet been published at that point. The later
thirty-candidate follow-up preserved that commit and published it to origin/main
under the user's explicit push instruction. No infrastructure deployment occurred.
