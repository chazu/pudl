# `pudl doctor` scan for likely secrets in stored data — pudl-udi part 2

Part 1 (`implog/2026_11_01_secret_pattern_detection.md`) added the detector
(`internal/secrets`), the schema-aware wrapper (`internal/secretscan`) and the
import and `facts add` warnings. This pass adds the doctor check over data
already stored. It is a check, not a repair: it reads and reports, and writes
nothing. Observe-ingest wiring remains open (see part 1).

## Behavior

- **Payloads.** The catalog scan (`cmd/doctor_catalog.go`) already loads each
  entry's payload (`loadVerifyData`) to validate it. The same decoded value is
  now scanned with `secretscan` under the entry's schema, so paths in the
  schema's `_pudl.sensitive_fields` or `_pudl.secret_scan_ignore` (inherited
  along `base_schema`) are not reported. Non-JSON documents (YAML, CSV,
  compressed) that `loadVerifyData` returns as raw text are decoded with
  `importer.DecodeStoredFile` first. Collection parents are skipped: each of
  their items is an entry and is scanned on its own.
- **Facts.** The args of every row in `current_facts` are scanned with no
  skip list (facts have no schema). Projection-owned facts (source
  `projection:…`) are excluded: projection copies values from a cataloged
  payload after redacting its sensitive fields, and that payload is scanned
  above under its schema's coverage. Scanning the copy would report the same
  value twice, under a fact path that `secret_scan_ignore` cannot suppress.
  The reserved `catalog_entry` relation is a view over catalog metadata and
  never stored in `current_facts`, so nothing needs excluding for it.
- **Scope.** `--entry`, `--schema` and `--origin` select catalog entries.
  Facts have no schema or origin, so a scoped run scans only the selected
  payloads and reports `facts_scanned: false`. `--health-only`, or a failed
  health check, skips the catalog scan and the `secrets` section is absent.
  After `--fix` changes entries, the re-run catalog scan rescans.
- **Streaming.** Entries are streamed from SQLite (`EachEntryContext`) instead
  of loading the whole catalog with `QueryEntries`; facts likewise
  (`EachCurrentFactContext`). The report keeps at most 500 findings
  (`doctor.MaxSecretFindings`) and counts all of them in `total`.
- **Exit status.** Findings are warnings: they never change `ok` or the exit
  status. Detection is heuristic (the entropy rule especially), so a false
  positive must not fail CI runs of `doctor`; and there is no in-tool fix for
  stored plaintext yet (the scrub is `pudl-m10`).
- **Values never appear.** Findings carry entry ID and proquint or fact ID,
  schema or relation, path and kind only.

## Output

Text, after the catalog lines (first 20 findings, then the total):

```
⚠️  Secrets: 2 likely secrets outside schema coverage (2 payloads, 1 facts scanned)
   entry babab-dodod [pudl/core.#Item]: auth.token (github_token)
   fact <id> (session): bearer (jwt)
   Fix: declare an entry's path in its schema's _pudl.sensitive_fields (a future scrub, pudl-m10,
   will redact the stored plaintext), or add it to _pudl.secret_scan_ignore if it is not a secret;
   route catchall records to a schema of their own with import --schema.
   Fact args have no schema: rotate the credential and retract the fact. Findings do not fail doctor.
```

With no findings: `Secrets: no likely secrets outside schema coverage (N
payloads, M facts scanned)`.

JSON: `report_version` is now **3** (version 2 had no `secrets`):

```json
"secrets": {
  "entries": 2, "facts": 1, "facts_scanned": true, "total": 2,
  "findings": [
    {"subject": "entry", "id": "…", "proquint": "…", "schema": "pudl/core.#Item",
     "path": "auth.token", "kind": "github_token",
     "fix": "Route the record to a schema of its own (import --schema), then declare auth.token in that schema's _pudl.sensitive_fields (a future scrub, pudl-m10, will redact the stored plaintext), or add it to _pudl.secret_scan_ignore if it is not a secret"},
    {"subject": "fact", "id": "…", "relation": "session", "path": "bearer", "kind": "jwt",
     "fix": "Fact args have no schema, so sensitive_fields and secret_scan_ignore do not apply: rotate the credential and retract the fact (pudl facts retract …); fact history keeps the value"}
  ]
}
```

An entry under a non-catchall schema gets `Declare <path> in <schema>'s
_pudl.sensitive_fields (…), or add it to _pudl.secret_scan_ignore if it is not
a secret`.

## API

`internal/doctor/secret_check.go`:

```go
const MaxSecretFindings = 500
type SecretFinding struct {
    Subject, ID, Proquint, Schema, Relation, Path string // Subject: "entry" | "fact"
    Kind secrets.Kind
    Fix  string
}
type SecretReport struct {
    Entries, Facts int
    FactsScanned   bool
    Total          int
    Findings       []SecretFinding
}
func NewSecretScan(src redact.Source) *SecretScan
func (s *SecretScan) Entry(entry database.CatalogEntry, data any)
func (s *SecretScan) Facts(ctx context.Context, db *database.CatalogDB) error
func (s *SecretScan) Report() SecretReport
```

`internal/database/catalog_stream.go`:

```go
func (c *CatalogDB) EachEntryContext(ctx context.Context, filters FilterOptions, fn func(CatalogEntry) error) error
func (c *CatalogDB) EachCurrentFactContext(ctx context.Context, fn func(Fact) error) error
```

`QueryEntriesContext`'s WHERE-clause builder moved unchanged into
`entryFilterClause`, shared with `EachEntryContext`.

`cmd`: `checkCatalogEntries(ctx, entryID, filter)` now streams, returns the
`*doctor.SecretReport`, and delegates each entry to `checkCatalogEntry`;
`doctorReport.Secrets` (`json:"secrets,omitempty"`); `printDoctorSecrets` in
`cmd/doctor_secrets.go`. Help text and `docs/cli/notes/doctor.md` describe the
scan; `docs/cli-reference.md` and the help golden are regenerated.

## Measurement

Run read-only against the local stores (counts only inspected): repository
`.pudl` 25 payloads and 5 facts, global `~/.pudl` 1166 payloads and 1678
facts; **0 findings** in both.

## Line deltas (`git diff --numstat` against `sre-focus` 200d1c3)

- Production Go: +325 / −88 (net +237). About 40 of the added lines are the
  WHERE-clause builder moved out of `catalog.go` (its −43).
- Tests: +96 (`cmd/doctor_secrets_test.go`).
- Docs: `docs/cli/notes/doctor.md`, regenerated `docs/cli-reference.md`,
  help golden, this implog, `docs/plan.md`.

## Tests

`cmd/doctor_secrets_test.go` `TestDoctorReportsStoredSecretsWithoutValues`:
a catchall entry holding a GitHub token is reported with entry ID, proquint,
path, kind and both remediations; a `user/vault.#Login` entry with the same
token at a `sensitive_fields` path and at a `secret_scan_ignore` path reports
nothing; a fact with a JWT in its args is reported by fact ID and relation; a
projection-owned fact holding a token is not scanned; `doctor` still exits 0
with `ok: true`; neither the JSON nor the text output contains either value;
`--schema` scans only matching payloads with `facts_scanned: false`;
`--health-only` omits `secrets`.
