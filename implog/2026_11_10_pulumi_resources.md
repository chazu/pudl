# Pulumi pack: pulumi.#Resource, facts and rules (2026-11-10)

Issue: `pudl-7g8j.4` (closed). Design: `docs/design/2026-11-06-pulumi-pack.md`
§5, §7, §9.

## What was built

- **Core: `#HTTPRun.records_optional`** (`internal/systemmodel/schema.cue`,
  `HTTPRun.RecordsOptional` in `internal/systemmodel/http_spec.go`,
  `runRecords` in `internal/populate/http_records.go`). When this is true, a
  response without the records path has no records. Otherwise it is a failed
  input, as before. A records path that is not an array still fails.
- **`pulumi.#Resource`** (`packs/pulumi/schema/pulumi/resource.cue`, closed,
  draft):
  - Fields: `urn`, `stack`, `type`, `id?`, `custom`, `external`, `protect?`,
    `parent?`, `dependencies?`, `created?`, `modified?`, `sourcePosition?`,
    `provider?` and `live?.metadata{uid?, name?, namespace?}`.
  - Identity is `urn`. The fact is `pulumi_resource(urn, stack, type,
    custom, external, parent, has_parent)`.
- **`pulumi.#Stack` fact**: `pulumi_stack_source(stack, repo, root, commit,
  dirty)`.
- **`_pulumiResourceTransform`**: an allow list. It keeps no inputs and no
  outputs except the `live` metadata of `kubernetes:*` types, and only when
  they are plain strings. The provider reference loses its `::<id>` suffix.
- **Models**:
  - `pulumi-resources` (`#HTTPObserve`, the same guard and stack enumeration
    (`_pulumiStacksEach`), `GET /api/stacks/{{item}}/export`, `records:
    deployment.resources`, `records_optional`, `reject_keys`, observation
    scope `pulumi/<org>/resources`).
  - `pulumi-fixture-resources`.
- **Rules** (`packs/pulumi/schema/pudl/rules/pulumi_pack.cue`):
  `pulumi_managed(urn, stack)` and the recursive `pulumi_ancestor(urn,
  ancestor)`. They match `has_parent: true`, because a missing fact
  argument reads as `null` in a join.
- **Fixtures**:
  - `resources-dev.json` is hostile. It has a kubeconfig provider, a Secret
    with a last-applied annotation and base64 data, a ConfigMap whose
    `metadata.name` is a secret envelope, a `command:local:Command` with env
    values and init errors, and a SQL user with a plaintext password.
  - `resources-prod.json` has a nested component chain and an external
    resource.
- **Docs**: the pack README (resources, facts and rules, and `settings.cue`
  as the settings file), the pudl-core skill (`records_optional`), and the
  design as-built notes.

## Tests

- `internal/populate`: `TestHTTPRunRecordsOptional`.
- `cmd/pulumi_pack_resources_test.go`:
  - `TestPulumiPackFixtureResources`: the exact kept fields, the envelope
    dropped from `live`, no planted value in any stored file (including
    the catalog DB), `pulumi_managed` and `pulumi_ancestor` results.
  - `TestPulumiPackStackSourceFacts`.
  - `TestPulumiPackLiveResources`: against the fake Pulumi Cloud, including
    an export with no resources array.
- `cmd/pulumi_pack_test.go`: shared helpers `pulumiLiveWorkspace` (settings
  in a top-level `schema/settings.cue`) and `pulumiLiveRun`.

## Live verification

In a scratch workspace outside the repo, with the read-only org token:
- The run succeeded with no failed inputs.
- The stored records had only the allow-listed keys, and `pudl doctor`
  passed with sensitive-scrub clean.
- No stored record holds `ciphertext`.
- `pulumi_managed` and `pulumi_ancestor` return rows, with no `null`
  ancestors.

Output was filtered to counts and key names, and the scratch workspace was
deleted.
