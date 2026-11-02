# Likely-secret detection outside sensitive-field coverage — pudl-udi

Schema redaction (`_pudl.sensitive_fields`) only protects records routed to a
schema that declares it. This pass adds a detector for likely secrets anywhere
else, and wires it into import and `pudl facts add`. Detection warns; it never
rewrites data, hashes or storage.

Out of scope for this pass (scheduled separately): the `pudl doctor` scan of
stored payloads and facts, and observe-ingest (`#CommandObserve`). Wiring for
both is described below.

## Detector: `internal/secrets` (pure)

No database, schema or importer dependencies (imports only `internal/fieldpath`).

```go
type Kind string
const (
    AWSAccessKeyID, AWSSecretAccessKey, GCPServiceAccountKey, GCPAPIKey,
    GitHubToken, GitLabToken, SlackToken, PrivateKey, JWT, URLCredentials,
    HighEntropy Kind = ...
)
const RedactedValue = "[REDACTED]" // equals redact.Placeholder (tested)
const RootPath = "(root)"

type Finding struct {
    Path string `json:"path"` // field path syntax, arrays as [*]
    Kind Kind   `json:"kind"`
}
func (f Finding) String() string          // "path (kind)"
type Options struct{ Skip []fieldpath.Path }
func Scan(v any, opts Options) []Finding   // sorted, one per (path, kind)
func ScanJSON(raw []byte, opts Options) ([]Finding, error)
func Describe(findings []Finding) string
```

- Findings carry the path and kind only, never the value or a substring.
- Paths use `fieldpath` syntax with `[*]` for array elements and JSON-quoted
  keys that contain `.`, `"` or `[`, so a path can be pasted into
  `sensitive_fields` or `secret_scan_ignore` as is. Elements of one array
  collapse into one finding.
- `Skip` drops findings at or below a covered path. Values equal to
  `[REDACTED]` are never reported.

Kinds:

| Kind | Rule |
|---|---|
| `aws_access_key_id` | `AKIA`/`ASIA` + 16 `[A-Z0-9]`, delimited (IAM unique IDs such as `AROA`/`AIDA` are not matched) |
| `aws_secret_access_key` | 40 chars of `[A-Za-z0-9/+]`, 3 character classes, under a key whose name contains `secret` |
| `gcp_service_account_key` | PEM private key in an object with `type: "service_account"` |
| `gcp_api_key` | `AIza` + 35 chars |
| `github_token` | `gh[pousr]_` + 36+ chars, `github_pat_` + 22+ |
| `gitlab_token` | `glpat-`, `gldt-`, `glrt-`, `glptt-`, `glft-`, `glcbt-`, `glsoat-`, `gloas-` + 20+ |
| `slack_token` | `xox[abposre]-`, `xapp-` + 10+ |
| `private_key` | PEM `-----BEGIN … PRIVATE KEY-----` header, or base64 of one (`LS0t…`, kubeconfig `client-key-data`) |
| `jwt` | `eyJ….eyJ….sig` |
| `url_credentials` | `scheme://user:password@`, except masked (`****`), `$VAR`, `{{…}}`, `%…`, `<…>` passwords |
| `high_entropy` | conservative heuristic, below |

High-entropy heuristic (`entropy.go`): length 20 to 4096; only
`[A-Za-z0-9+/=_-]` (no spaces, dots, colons, `@`, so URLs, ARNs, image refs,
timestamps and JSON text are out); not hex (digests), not a UUID; field name
has no word in {sha, sha1, sha256, sha512, md5, crc32c, digest, etag,
fingerprint, checksum, hash, id, ids, uid, uuid} (camelCase and `_`/`-` split,
so `labelFingerprint`, `md5Hash`, `private_key_id` are skipped); a value with
`/` but no `+`/`=` is judged by its longest segment (paths, `owner/repo`
slugs); lower, upper and digit all present; Shannon entropy at least
`min(4.3, 0.92·log2(n))` bits/char; and not base64 of printable text (CA
certificates, GitHub node IDs).

## False-positive measurement

- Repository fixtures (`test/data`, `test/integration/testdata/type_detection`,
  `examples/*/*.json`) plus a new realistic corpus
  `internal/secrets/testdata/cloud/{gcloud,aws,kubectl}.ndjson` (compute
  instances and firewalls, GKE cluster with base64 CA, service accounts and
  keys, Cloud Run, GCS object, IAM policy, projects, Secret Manager versions;
  EC2 describe-instances, IAM role/user, STS identity, Lambda, ECR, CloudFormation,
  Secrets Manager, EKS, KMS, CloudFront, Cognito; pods, nodes, ConfigMap,
  Deployment with last-applied-configuration, kubeconfig, Service): 23 files,
  76 records, 1137 string values, **0 findings**. Guarded by
  `TestNoFindingsInCloudFixtures`.
- The local stores (`~/.pudl/data/raw`, the repo's `.pudl/data/raw`; linux
  packages/services/users, GitHub repositories, mu snapshots): 2758 records,
  19057 strings. First run: 1 finding (a GitHub `full_name` slug,
  `owner/MixedCaseName`), which led to the longest-segment rule; after it,
  **0 findings**. Only counts, paths and kinds were inspected.
- Sensitivity of the entropy heuristic on random tokens (2000 each, key
  `value`): base62 n=20 70%, 24 63%, 32 96%, 40 99.8%, 64 100%; base64
  n=20 57%, 24 55%, 32 85%, 40 93%, 64 99%. Short tokens are deliberately
  missed rather than risk identifiers.

## Suppression: `_pudl.secret_scan_ignore`

```cue
_pudl: secret_scan_ignore: ["spec.nonce", "status.conditions[*].token"]
```

Decoded strictly next to `review` (`internal/validator/secret_scan.go`,
called from `decodeStrictPudlParts`) into `SchemaMetadata.SecretScanIgnore`.
A non-concrete value, a non-list, non-strings or an unparsable path set
`SchemaMetadata.SecretScanIgnoreError`; nothing is suppressed and
`CheckModuleIntegrity` reports `schema X has invalid _pudl.secret_scan_ignore:
…` as a schema load error. Inherited along `base_schema` like
`sensitive_fields`.

## Schema-aware scanning: `internal/secretscan`

```go
func New(src redact.Source, sensitive *redact.Registry) *Scanner // nil sensitive builds one
func (s *Scanner) Scan(record any, schemas ...string) []secrets.Finding
func (s *Scanner) Covered(schemas ...string) []fieldpath.Path
```

`Covered` is the union, along each schema's base_schema chain, of
`sensitive_fields` and `secret_scan_ignore` (cached per schema). A broken
sensitive declaration covers nothing (redaction already refuses those records);
a broken ignore list suppresses nothing.

## Import integration

- `EnhancedImporter.secretScan` is built beside the redactor
  (`secretscan.New(inferrer, imp.redactor)`).
- Each record is scanned after schema assignment and redaction, with the
  intended (`--schema`) and assigned schemas: collection items in
  `collectionStream.prepareAssigned`, documents in `importDocument`, and every
  record in `Preview` (`--dry-run`). The scan only reads the decoded record.
- `ImportResult` gains `secret_warnings` (`[]SecretWarning{record, schema,
  path, kind}`, capped at 50) and `secret_warnings_total`, part of the
  `pudl import --json` outcome. `internal/importer/secret_report.go` holds the
  tally.
- Human output (`cmd/import_warnings.go` `secretNotes`, shown in the import
  summary, batch progress and `--dry-run` preview): a count, one line per
  finding `record N (schema): path (kind)`, and the fix — add the path to
  `_pudl.sensitive_fields` to redact it, or to `_pudl.secret_scan_ignore` if
  it is not a secret; route catchall records with `--schema`.
- Not scanned: a document whose content hash is already cataloged (the import
  is skipped before decoding). Doctor's stored-payload scan covers that.

## Fact writes

- `pudl facts add` warns on stderr after the write: `args contain likely
  secrets: path (kind)` and that facts are stored unredacted and kept in
  history. `--json` stdout is unchanged.
- `pkg/factstore` (additive, existing calls compile unchanged):

```go
type SecretFinding = secrets.Finding
type SecretError struct{ Relation string; Findings []SecretFinding }
type Option func(*Store)
func Open(pudlDir string, opts ...Option) (*Store, error)
func WithSecretRejection() Option        // Store.AddFact returns *SecretError; nothing written
func ScanFactArgs(args string) ([]SecretFinding, error)
```

  Rejection is off by default and does not cover `Tx.AddFact` inside
  `Transact` (documented; call `ScanFactArgs` there).

## Wiring the deferred integrations

**`pudl doctor` (stored payloads and facts).** In the doctor check:

```go
scan := secretscan.New(inferrer, nil) // inference.Shared(schemaPaths...)
for each catalog entry (items, documents; skip collection parents):
    data, _, err := importer.DecodeFile(entry.StoredPath, entry.Format)
    for _, f := range scan.Scan(data, entry.Schema) { report entry.ID, f.Path, f.Kind }
for each current fact:
    findings, _ := secrets.ScanJSON([]byte(fact.Args), secrets.Options{})
    report fact.ID, fact.Relation, findings
```

Report IDs, paths and kinds only. Stored payloads routed to a schema whose
`sensitive_fields` were declared later are still plaintext; `Scan` skips
those paths, so add `Options{}` (no skip) or report them separately if doctor
should flag retroactive exposure.

**Observe ingest (`#CommandObserve`, moving to `internal/observe`).** Where the
route applies redaction (`redact.Apply` after `redactor.For(intended,
assigned)`), add:

```go
findings := scanner.Scan(record, intended, assigned) // scanner := secretscan.New(inferrer, route.redactor)
```

and append `{record index, schema, findings}` to the observe result the same
way `FactWarnings` is reported (an `[]importer.SecretWarning`-shaped field),
then print it with the import's `secretNotes` wording.

## Line deltas (`git diff --numstat` against `sre-focus` 46ab851)

- Production Go: +753 / −11 (net +742). New: `internal/secrets` 434,
  `internal/secretscan` 86, `pkg/factstore/secrets.go` 53,
  `internal/importer/secret_report.go` 52, `internal/validator/secret_scan.go`
  44; wiring in importer, validator, cmd, factstore 84 / −11.
- Tests: +517 Go test lines, plus 28 lines of NDJSON corpus.
- Docs: +51 / −6 (`projection.md`, `library-api.md`, import notes,
  regenerated `cli-reference.md`).

## Tests

- `internal/secrets`: every kind detected; contextual kinds (service account,
  AWS secret beside a secret-named key); output never contains the value;
  paths parse as field paths (quoted keys, `[*]`, nested arrays, root);
  skip/redacted handling; identifier and digest non-findings; fixture corpus
  has zero findings.
- `internal/validator`: `secret_scan_ignore` decode table (valid, not a list,
  non-strings, bad path, non-concrete), load-error reporting.
- `internal/secretscan`: placeholder agreement with `redact`; scan without a
  schema source.
- `internal/importer`: catchall NDJSON with a GitHub token warns with record
  index and path, JSON has no value, stored bytes and content hash unchanged;
  document URL credentials; sensitive (child) + inherited ignore (base) leave
  only the uncovered path, preview and import agree; warning cap.
- `pkg/factstore`: accepted by default; `WithSecretRejection` returns
  `*SecretError` without the value and writes nothing; `ScanFactArgs`.
- `cmd`: `import --dry-run` notes and `--json` `secret_warnings`, no value in
  any stream; `facts add` stderr warning with clean `--json` stdout.
