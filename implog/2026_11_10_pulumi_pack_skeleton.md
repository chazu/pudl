# Pulumi pack skeleton: settings, token guard, pulumi.#Stack (2026-11-10)

Issue: `pudl-7g8j.3` (closed). Design: `docs/design/2026-11-06-pulumi-pack.md`
§2–4.

## What was built

- **Pack `pulumi`** (`packs/pulumi/`, registered in `internal/packs`):
  `bundle.go`, `README.md`, `schema/pulumi/stack.cue`,
  `schema/models/pulumi_pack.cue`, `schema/models/pulumi_guard.cue`, and
  fixtures in `populators/pulumi/`.
- **Settings** (package `models`): `_pulumiOrg` and `_pulumiTokenName` with no
  default; `_pulumiAPI` with default `https://api.pulumi.com`.
- **Token guard** (`_pulumiGuard`): four GET requests (user, assigned role,
  policies, sets). It passes only for the named org token of the org (with
  `team: null`), with exactly one role. Every node of the role's tree, and of
  the policies and sets it names, must be known, and every scope that it
  reaches must be in `_pulumiReadScopes`. The walk is non-recursive
  (`_#PulumiNode`, `_#PulumiTree`, 4 levels by 3 ID rounds): CUE 0.16
  reports a false cycle in a recursive walk.
- **`pulumi.#Stack`** (closed schema, draft): `stack`, `version`, `kind`,
  `result`, `startTime`, `endTime?`, `resourceCount?`, `pulumiVersion?`,
  `source{repo?, vcs?, root?, commit?, branch?, dirty}`.
- **Models**: `pulumi-stacks` (`#HTTPObserve`, auth `PUDL_PULUMI_TOKEN` with
  scheme `token`, stack list paging, `updates?pageSize=1&page=1`,
  `reject_keys` for secret envelopes, observation scope
  `pulumi/<org>/stacks`); `pulumi-fixture-stacks` (`cat` of the update
  fixtures, same `_pulumiStackTransform`).
- **CLI text**: `pudl pack install` help and its "Next:" hint, `pudl guide`,
  `docs/cli-reference.md`, `cmd/testdata/help.golden.json`.

## Tests

- `cmd/pulumi_pack_test.go`:
  - `TestPulumiPackFixtureStacks` checks the stored fields and searches
    every stored file for the planted values.
  - `TestPulumiPackGuard` runs the live model against an `httptest` TLS
    server. It checks paging and a role that passes through a policy
    condition. The refusal cases are personal, team, another-org and
    another-name tokens, no role, two roles, decrypt or write scopes
    (inline and behind a policy condition), an unknown descriptor, an
    unknown ID and an ID cycle. Each refusal sends only the four guard
    requests.
- Updated `cmd/pack_test.go` and `internal/packs/packs_test.go` for the
  third pack.

## Live verification

In a scratch workspace outside the repo, with the read-only org token: the
guard passed, and every stack was read with no failed input. The stored
records had only the allow-listed keys, and `pudl doctor` passed. Output
was filtered to counts, and the scratch workspace was deleted. No org
value is in the repo.
