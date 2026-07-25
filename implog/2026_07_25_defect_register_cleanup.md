# Defect register cleanup: provenance, scope errors, re-ingest, scratch dirs

Clears the remaining defect-register backlog after the `--only` scoping cluster.
Six issues closed, one partially closed with its scope corrected.

| Issue | Defect | Report # | Outcome |
|-------|--------|----------|---------|
| `pudl-emi` | Run/observation association is last-writer-wins | 7 | fixed |
| `pudl-sln` | `GetCollectionByID` failure reports every resource missing | 13 | fixed |
| `pudl-6pf` | Manifest re-ingest skips status writes, wrong run ID | 12 | fixed |
| `pudl-fue` | Snapshot content-hash dedup is dead code | 11 | fixed |
| `pudl-j4t` | Killed run leaves `pudl_run_*` in the mu project root | 6 | fixed |
| `pudl-psd` | Dead code: two unreferenced ingest wrappers | 15 | fixed |
| `pudl-xlx` | No shared transaction boundary within a run | 14 | **partial** |

## pudl-emi — observations keep the run that first saw them

`ingestObserveRecord` called `UpdateEntryRunID` on every deduplicated record, so
an entry first observed by run A silently moved to run B on the next identical
observation. Querying run A then under-reported what run A saw, and the entry's
`run_id` disagreed with its snapshot membership about the same fact — two durable
identifiers telling different stories, which is precisely what invariant 3
forbids and what re-running (D1) must not degrade.

The overwrite is simply gone. The current run's sighting is recorded as snapshot
membership, which is the relationship that is legitimately many-to-many, so
nothing is lost. `UpdateEntryRunID` had no other caller and was deleted with its
file — leaving the operation available would invite the same defect back.

## pudl-sln — a DB fault is not an observation

`loadObservedRecords` treated any `GetCollectionByID` failure as "not a snapshot
ID, try it as an origin". For a genuine database error that fallback matches
nothing, hands the set-diff an empty observed set, and reports every desired
resource `missing` — under `--converge`, re-applying the entire model off a
transient fault.

Only `ErrCodeNotFound` now falls back; anything else is fatal. The classification
is extracted as `observeScopeFilter(scope, lookupErr)` because it is otherwise
untestable: a closed database fails *every* query, so the buggy and fixed paths
both end in an error and the reproduction proves nothing. As a pure function of
the lookup error, the DB-error case is asserted directly.

## pudl-6pf — what a duplicate manifest may safely write

The early return skipped the per-action `UpdateStatus` loop and returned the
original run's ID. Both halves needed care rather than a straight "do it anyway":

**Statuses.** The content hash covers the manifest's own `timestamp`, so two
distinct applies never collide — a duplicate is the *same apply* recorded twice,
and its statuses were already written. Rewriting them wholesale would knock a
resource the drift re-check has since promoted to `clean` back to `converging`,
undoing a verification with information older than it. But there is a real hole:
the first ingest treats an `UpdateStatus` failure as a warning, so an action's
apply can be recorded while its resource sits at the default `unknown`. A
re-ingest now repairs exactly those rows and touches nothing else, reporting the
count as `StatusesRepaired`.

**Run ID.** Not changed to the caller's, deliberately. The manifest entry is
content-addressed and its run association is first-writer-wins — rewriting it
would be `pudl-emi` all over again. Instead the ambiguity is removed: the field
is documented as the *owning* run, and the CLI now says "already recorded by
run_id: X" rather than implying it is yours.

## pudl-fue — the dedup lookup is dropped, not repaired

The hashed snapshot payload carries its own `snapshot_id`, `timestamp` and
`run_id`, so `GetEntry(contentHash)` could never hit. The ticket offered two
routes; content-only hashing is the wrong one. A snapshot is the record of *one*
observation by *one* run, which is what invariant 3 requires and what
`--catalog-scope` selects — collapsing two observations into a shared row would
leave the second run with no snapshot of its own. The lookup is removed and the
reasoning recorded where it was.

## pudl-j4t — scratch directories

Two halves, one unfixable:

- **Signals.** `setupReconcileWorkspace` now installs a SIGINT/SIGTERM handler
  that removes the workspace, uninstalls itself, and re-raises so the process
  still dies of the signal it was sent rather than reporting a plain exit status.
- **Sweep.** Each run first removes `pudl_run_*` directories older than 24h from
  the mu project root, collecting what earlier runs leaked. The age gate is what
  makes this safe alongside a concurrently running pudl: another process's
  workspace is only touched if that run has been going longer than a day.
- **SIGKILL** cannot run cleanup. The sweep is what eventually reclaims those.

The cleanup function is now idempotent (`sync.Once`) because setup calls it on
every failure path and the caller also defers it.

## pudl-xlx — partial, and the ticket's target was wrong

The recorded symptom is fixed: the inventory path opened the catalog *before*
`runPopulate`, holding a reader across the writer that produces the very records
it then reads. The open moved after populate; nothing there reads the catalog
earlier.

The recommendation itself is untouched, and the ticket has been rescoped rather
than closed. Working on it surfaced that **"one transaction per run" should not
be built**: a converge run shells out to mu for minutes, so a write transaction
spanning the run would hold the catalog locked for its whole duration — against
other invocations and against the run's own second handle. The atomic unit is the
*step* — an observation plus its snapshot membership, or a convergence step's
manifest plus per-action statuses — which is short and holds no lock across a
subprocess. Recorded in Recommendation 4, along with the outstanding precondition
(one run-owned handle replacing six independent `NewCatalogDB` opens, carrying
the best-effort semantics explicitly rather than by accident).

## Public API

```go
// internal/database — REMOVED (the operation was the defect)
func (c *CatalogDB) UpdateEntryRunID(entryID, runID string) error

// internal/mubridge — result gains a repair count; RunID semantics documented
type IngestManifestResult struct {
	RunID            string // the run that OWNS the manifest; not the caller's when Skipped
	Total, Cached, Failed int
	Skipped          bool
	StatusesRepaired int    // new
}

// internal/mubridge — new, unexported
func actionTargetName(action ManifestAction) string
func actionStatus(action ManifestAction) string
func repairMissingActionStatuses(db *database.CatalogDB, manifest ManifestInput) (int, error)

// cmd — new, unexported (cmd/run_workspace_cleanup.go)
const workspacePrefix = "pudl_run_"
const staleWorkspaceAge = 24 * time.Hour
func removeOnSignal(dir string) func()
func sweepStaleWorkspaces(muRoot string, maxAge time.Duration) []string
func reportSweptWorkspaces(removed []string, live bool)
func observeScopeFilter(scope string, lookupErr error) (collectionID, origin string, err error)

// cmd — REMOVED (unreferenced)
func ingestObserveOutput(observeJSON []byte) (int, error)
func ingestObserveOutputWithSnapshot(observeJSON []byte) (int, string, error)
```

## Tests

- `TestIngestObserveResultsWithSnapshotRunID_AttachesAuditIdentity` — rewritten;
  it previously *asserted the defect* (`run_next` stealing the record). Now
  asserts the record keeps `run_test`, each run gets its own snapshot, and the
  shared entry belongs to both snapshots.
- `TestObserveScopeFilter` — found / not-found / DB-error, the last asserting a
  fault is fatal rather than an empty observed set.
- `TestIngestManifest_DedupDoesNotRegressVerifiedStatus` — a `clean` resource
  survives a duplicate ingest.
- `TestIngestManifest_DedupRepairsUnwrittenStatus` — an `unknown` row is repaired.
- `TestIngestManifest_DedupReportsOwningRun` — a duplicate names the earlier run.
- `TestRemoveOnSignal_{CleanupRemovesDir,CleanupIsIdempotent,RemovesDirOnSignal}`
  — the signal case runs in a **subprocess**, since the handler re-raises against
  the default disposition and would otherwise kill the test binary. Verified
  non-vacuous: pointing the handler at a different signal makes it fail.
- `TestSweepStaleWorkspaces` — stale collected; a fresh (concurrent-run)
  workspace, an unrelated directory, and a file sharing the prefix all survive.
- `TestSweepStaleWorkspaces_MissingRootIsNotAnError`.

Full suite green (`CGO_ENABLED=0 go test ./...`), `go vet ./...` clean. The
`smoke`-tagged tests were not run — they need docker/k3d/kubectl/mu.
