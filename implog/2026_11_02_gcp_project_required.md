# GCP records must carry a valid project ID; `@pudl(match)`; `tracked_fields` removed

Follow-up to `pudl-8wr`. Every GCP pack type already required `project:
string`, but a record without a usable project still got into the catalog:

- **Without `--schema`, a record lacking `project` imported anyway**, as
  `pudl/infra.#Service` (or `core.#Item` for collection items), because it
  failed its pack type and inference fell back to a generic schema.
- **`--set project=` (empty), a project number or a display name passed**,
  because `string` accepts any string.

## Changes

**`project` is a project ID.** New `examples/gcp/schema/gcp/project.cue`
defines `_gcpProjectID`: 6–30 lowercase letters, digits and hyphens, starting
with a letter and not ending with a hyphen, with an optional legacy domain
prefix (`example.com:my-project`). All 15 GCP pack types and the
gcp-network-hygiene `#Firewall` use it. It is the ID the collectors pass to
`gcloud --project`; the project number (all digits) and display names (spaces,
capitals) are rejected. A display name that is itself a valid ID cannot be
told apart.

**`@pudl(match)` claims records for a type.** A type marks the body fields
that only its records carry with the field attribute `@pudl(match)`
(required fields, any struct depth). A record whose values at every marked
field satisfy the schema's own constraint there claims the type, and the claim
extends to the type's specializations. Inference considers only claimed types
for such a record; if none unifies it returns `inference.ClaimError` instead
of falling back. Every GCP type marks one or more fields: `kind` for Compute,
DNS and SQL; `selfLink` (now constrained to the GKE URL shape in the body) for
GKE; `name` (already a path pattern) for Pub/Sub and KMS; `storage_url` for
buckets; and `apiVersion`, `kind` and the `cloud.googleapis.com/location`
label for Cloud Run, so Knative services from other clusters are not claimed.
The policy engine accepts and ignores the `match` key.

A first version declared `_pudl.match: {path: regex}`. It restated
constraints the schema body already held, so it was replaced by the attribute
before commit; the body constraint is the single source of truth.

`@pudl(match)` also replaces the automatic pinned-literal detection added for
`pudl-8wr` earlier today (`validator.literalStringFields`,
`SchemaMetadata.Discriminators`, `inference.matchesDiscriminator`), which were
deleted. Automatic detection would have claimed unrelated records, such as any
JSON with `kind: "kit"` (the bootstrap `brick` schemas pin it), so claiming is
opt-in.

**`tracked_fields` removed.** It only added up to +0.1 to an inference score
when its field names were present, overlapping with `@pudl(match)` as the
"marks the type" signal, and no content pack used it. Removed from
`SchemaMetadata`, inference scoring, the import metadata files
(`ResourceTracking`, `SchemaRegistryEntry`), `schema new` output, `schema list
--verbose`, the bootstrap and template schemas, and the docs. A leftover
`tracked_fields` key in a workspace schema is ignored.

**Claim failures are never swallowed.**

- Collection items: `assignItemSchemaDetailed` no longer turns a
  `ClaimError` into the `core.#Item` catchall.
- Observe ingest: a collector without `schema:` routed records by `_schema`
  or to the generic observe schema without inference. A record that claims a
  type is now inferred, and rejected on failure.
- Doctor: an existing entry that claims a type and fails it reports `error`.

**CLI errors show their cause.** `errors.CLIErrorHandler` printed only a
wrapped error's message ("Failed to import file") and hid the cause unless in
verbose mode, which nothing enabled. It now prints the cause on the error line,
for every command:
`Error: Failed to import file: record 0: failed to infer schema: record matches
gcp.#Firewall (@pudl(match)) but does not satisfy it: #Firewall.project: ...`.

## Public API

- `validator.SchemaMetadata.Match []MatchRule`, `MatchError string`;
  `validator.MatchRule{Path, Constraint}`; `SchemaMetadata.Claims(data) bool`.
- `inference.ClaimError`; `(*SchemaInferrer).Claims(data) bool`;
  `CandidateScore.Claimed`.
- Removed: `SchemaMetadata.Discriminators`; `TrackedFields` on
  `validator.SchemaMetadata`, `importer.ResourceTracking` and
  `importer.SchemaRegistryEntry`.

## Tests

- `internal/validator/match_test.go`: marked fields at the top level and
  nested (shorthand), optional fields ignored, non-matching and wrong-typed
  values, and a malformed attribute.
- `internal/inference/claim_test.go`: claimed assignment, inherited claims,
  name-pattern claims, unclaimed fallback, and rejection for a missing
  project, a project number and a display name.
- `internal/observe/claim_route_test.go`: schema-less observe routing.
- `cmd/gcp_pack_inference_test.go`: every GCP fixture is rejected without a
  valid project ID (empty, number, display name, and absent where gcloud omits
  it) and classifies as its pack type with one.

## Validation

`go test ./...`, `make test-race`, `make lint` (0 issues), `make
test-git-walkthrough`, `make check-docs check-skills`: pass. By hand with a
fresh binary: firewalls.json is rejected with no project, an empty project, a
project number and a display name, and with `--schema gcp.#Firewall`; with
`--set project=example-project` all five records are `gcp.#Firewall`.
