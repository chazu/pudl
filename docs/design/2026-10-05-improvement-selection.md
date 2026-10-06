# Improvement selection and delivery

This review starts from `b3a9b34`, including the two October 5 improvement
rounds already present in the checkout. Existing behavior is evidence, not a
reason to implement the same feature again. The selection favors demonstrated
failures, preservation of PUDL's ownership boundaries, and small public contracts.

## Thirty candidate ideas

1. Reject malformed and contradictory query inputs before opening the catalog.
2. Confine orphan file removal with filesystem directory handles.
3. Enforce the bundle byte budget while copying the SQLite snapshot.
4. Give bundle export and restore the same manifest size limit.
5. Return output failures from every query presentation mode.
6. Add a browser dashboard for observations and approvals.
7. Add a background daemon to refresh inventories continuously.
8. Execute provider actions directly from PUDL.
9. Add automatic inverse-action rollback after failed convergence.
10. Restore pending approvals as immediately executable plans.
11. Stream JSON collection imports with exact numeric tokens.
12. Add current, previous, and expected values to drift reports.
13. Add bounded failed-check witnesses to reports.
14. Introduce independent snapshot retention owners.
15. Add portable verified workspace bundles.
16. Explain schema inference decisions on demand.
17. Add a persistent compiled-schema cache between invocations.
18. Replace SQLite with a network database.
19. Reintroduce the bundled agent memory application.
20. Add a new service framework around the run coordinator.
21. Add retries to every failed Mu operation.
22. Add native YAML and CSV collection streaming.
23. Parallelize model execution automatically.
24. Add indexes to every frequently mentioned catalog column.
25. Make content IDs available in a shorter display format.
26. Raise the recursive query iteration cap substantially.
27. Add more format parsers to the import pipeline.
28. Require infrastructure smoke tests for every change.
29. Add signed bundles and creator identity verification.
30. Rewrite the CLI as a new command hierarchy.

## Critical evaluation of each candidate

| # | Decision | Evidence and reason |
|---|---|---|
| 1 | Keep | `query.go` ignores arguments without `=`, accepts empty keys, and replaces duplicate keys. `--list` ignores positional arguments and `--topo`; negative iteration caps silently become defaults. These change the requested question without informing its author. |
| 2 | Keep | `removeCommittedOrphanAt` checks a lexical prefix, then calls `os.Remove`. A symlink in a parent directory can redirect deletion outside raw/metadata. Directory handles can enforce the boundary at removal. |
| 3 | Keep | `Export` calls `onlineSnapshot` before comparing the copied database size with `maxBytes`. A tiny caller budget still permits a large database copy. Check source size and destination growth during incremental backup. |
| 4 | Keep | Restore rejects manifests over 8 MiB, while `writeArchive` emits manifests of any size. A large inventory can therefore produce a successful backup its own reader rejects. |
| 5 | Keep | Query JSON, tuples, list and topology output ignore writer errors. A failed consumer can receive incomplete data while the command reports success. |
| 6 | Reject | Adds a second product and deployment surface without a demonstrated workflow that the existing CLI reports cannot serve. |
| 7 | Reject | Requires scheduling, ownership, locking and credential lifetimes; no current requirement justifies that operating cost. |
| 8 | Reject | Duplicates Mu's execution ownership and splits the provider contract. |
| 9 | Reject | Inverse actions cannot generally reconstruct external state, especially after a partial action. A generic promise would be misleading. |
| 10 | Reject | Restored history is deliberately inert; resurrecting execution authority invalidates fresh-plan and observation requirements. |
| 11 | Reject | Already delivered in `internal/ingestprep` and collection preparation with exact numbers and explicit budgets. |
| 12 | Reject | Already delivered through frozen drift findings and previous-baseline compatibility checks. |
| 13 | Reject | Already delivered through deterministic bounded witnesses and sealed-reference redaction. |
| 14 | Reject | Already delivered through manual, approval and report pins. |
| 15 | Reject | Already delivered. Improve its demonstrated resource and interoperability gaps in ideas 3 and 4. |
| 16 | Reject | Already delivered by `import --explain` and persisted assignment reasons. |
| 17 | Reject | File fingerprinting, CUE context identity and invalidation would become cross-process concerns without measured startup evidence. |
| 18 | Reject | PUDL's workspace-local, portable state is an advantage; a server requirement would undermine it. |
| 19 | Reject | Conflicts with the explicitly completed memory-application removal; the generic facts/Datalog substrate remains. |
| 20 | Reject | Existing run dependencies and the ACUTE coordinator already provide test seams; another framework adds indirection. |
| 21 | Reject | Retrying mutations without provider idempotency contracts can duplicate effects and obscure the approved operation. |
| 22 | Reject | Valuable for a demonstrated large YAML/CSV workload, but whole-document limits already bound those formats and there is no benchmark motivating parser complexity now. |
| 23 | Reject | Producer dependencies, exact approval and snapshot authority require deliberate scheduling semantics; there is no measured bottleneck to justify changing them. |
| 24 | Reject | Index cost belongs to a measured query plan and workload, not column popularity; writes and bundle size would increase. |
| 25 | Reject | Existing proquint IDs are the human surface; another representation would complicate resolution and collision handling. |
| 26 | Reject | A larger cap delays detection of nonconvergence and does not fix recursive semantics; the current cap is explicitly configurable. |
| 27 | Reject | Existing JSON/NDJSON/YAML/CSV and compression cover current examples. Each new parser adds inference, limits and export contracts. |
| 28 | Reject | Docker/Kubernetes qualification requires environment and credentials; hermetic tests and real-Mu local smoke already cover the default gates. |
| 29 | Reject | Bundle checksums explicitly prove integrity, not authorship. Signing needs a key distribution and trust model the project has not requested. |
| 30 | Reject | Command consolidation and generated help/reference already shipped; another hierarchy would disrupt working automation without a concrete usability failure. |

## Actionable plans for retained ideas

### 1. Validate the complete query request — confidence 99%

Extract constraint parsing into `parseQueryConstraints(args)`. Require `key=value`
with a nonblank key, reject repeated keys instead of overwriting them, and retain
existing exact-number and quoted-string parsing. Empty values and embedded equals
remain valid strings. Validate negative `--max-iterations`, `--list` positional
arguments, and `--list --topo` before catalog work. Keep zero as the existing default
iteration sentinel. Verify command-level invalid requests and valid numeric queries.

```go
key, raw, ok := strings.Cut(arg, "=")
if !ok || strings.TrimSpace(key) == "" { return nil, fmt.Errorf("expected field=value") }
if _, exists := constraints[key]; exists { return nil, fmt.Errorf("duplicate constraint %q", key) }
```

Benefit: an automation typo cannot silently broaden or replace a filter. Downside:
callers relying on ignored tokens or last-value-wins constraints must fix their
requests. Field names remain unrestricted because fact arguments are not limited
to Go identifiers. This does not introduce schema-based field validation.

### 2. Enforce the cleanup boundary during unlink — confidence 99%

Keep the SQL reference check and raw/metadata lexical selection. Open the owning
workspace with `os.OpenRoot`, then the selected artifact subtree with
`Root.OpenRoot`. Remove the relative name through that subtree's `Root.Remove`.
For prune's configurable data directory, establish the workspace handle first
for paths inside the workspace, and honor an explicitly supplied external data
directory as its own boundary. Missing files remain harmless; escaping
directory symlinks fail and retain the external file. Test both a nested escape
and a symlink replacing the artifact subtree, plus ordinary shared-file cleanup.

```go
workspace, err := os.OpenRoot(c.configDir)
artifactRoot, err := workspace.OpenRoot(relativeArtifactRoot)
err = artifactRoot.Remove(relativeFile)
```

Benefit: the filesystem enforces the documented ownership boundary instead of
trusting a string prefix. Downside: intentionally external symlinked storage cannot
be reclaimed automatically; operators must manage it themselves. This protects
against symlink escapes, not administrative bind mounts or malicious replacement
of the entire owning workspace before it is opened.

### 3. Bound SQLite backup staging — confidence 96%

Pass `maxBytes` into `onlineSnapshot`. Read source `page_count` and `page_size`
through the backup connection and reject an already oversized source before
creating a destination. After every 128-page backup step, check the destination
size and fail on excess; propagate cancellation as before. Compare by division
to avoid size multiplication overflow. Keep the existing aggregate file budget
for the remainder of the bundle. Test oversized rejection without destination
creation and exact-boundary backup preserving SQLite data.

```go
if pages > maxBytes/pageSize { return fmt.Errorf("catalog exceeds bundle byte limit") }
more, err := backup.Step(128)
// Stat destination and reject Size() > maxBytes before another step.
```

Benefit: a requested backup budget applies during its most expensive preparation
step. Downside: a growing live source may pass preflight and fail later; temporary
overshoot is checked after each backup step, and callers still need room for private
staging, SQLite journals and the final archive. This is a payload disk budget, not a total process-memory
or compressed-output guarantee.

### 4. Make manifest limits symmetric — confidence 98%

Define `maxManifestBytes` once and use it in both writer and reader. Serialize and
validate the manifest before creating the output temporary file or compressor.
Reject excessive metadata explicitly and preserve existing destinations. Add a
regression that attempts an oversized manifest against an existing destination
and verifies its bytes survive. Run the existing bundle round-trip suite.

```go
body, err := json.Marshal(m)
if len(body) > maxManifestBytes { return fmt.Errorf("bundle manifest exceeds size limit") }
```

Benefit: successful writer output satisfies the reader's framing limit. Downside:
extremely large inventories require smaller workspaces or a future versioned
manifest format; silently generating unusable recovery artifacts is worse.

### 5. Propagate query output failures — confidence 99%

Use `printJSON` for query JSON. Return tuple marshal/write failures, empty-result
write failures and final count write failures. Build relation-list text after
successful catalog/rule reads, then return its writer error. Propagate topology
writes as well. Inject a failing result writer and exercise JSON, human tuples,
empty results, lists and topology; normal numeric output must remain unchanged.

```go
if _, err := fmt.Fprintf(outw(), "%s(%s)\n", t.Relation, args); err != nil { return err }
return printJSON(rows)
```

Benefit: scripts cannot treat truncated query results as successful data delivery.
Downside: stdout can already contain a prefix when it fails, as with export; callers
must respect the exit status. This is intentionally scoped to every query mode,
not a speculative rewrite of all command renderers.

## Delivery evidence

Each retained item will receive its own implementation commit and implementation
log. The existing unpublished integrated evidence commit will be preserved and
published along with this work; it predates this selection. Completion requires
regressions for every retained item, whole-project tests, vet/build, generated
docs/skills verification, lint, and confirmation that local main equals remote
main. Race and smoke results will be recorded with their actual scopes.
