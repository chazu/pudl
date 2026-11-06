# Design: Pulumi pack (2026-11-06)

Epic: `pudl-7g8j`. Design (accepted):
`docs/design/2026-11-06-pulumi-pack.md`.

## Decided with the user

1. pudl uses only a read-only Pulumi Cloud token, from `PUDL_PULUMI_TOKEN`,
   and refuses any other token. Never the `pulumi` CLI or its credentials.
2. Collectors are CUE. No shell helpers, no `jq`. A new core source,
   `#HTTPObserve`, sends GET requests only; not `tools/flow`.
3. Allow list, not deny list, for state and update history.
4. Pulumi records and components draw no graph node of their own.
5. Repo link from update history (`vcs.repo`, `vcs.root`), no repo scan.

## Planned public surface (not implemented yet)

- `sm.#HTTPObserve` (`base`, `auth.env`, `guard`, `each` with `page`,
  `runs` with `records`, `reject_keys`).
- Pack settings `_pulumiOrg`, `_pulumiTokenTeam`.
- Schemas `pulumi.#Stack` (identity `stack`) and `pulumi.#Resource`
  (identity `urn`).
- Facts `pulumi_resource`, `pulumi_stack_source`; rules `pulumi_managed`,
  `pulumi_ancestor`; identity kind `gcp.resource`.
- `_pudl.graph` flag `member_only`.

## Verified while designing

- Update history `environment` keys, with no `git.remote`.
- Endpoint paths and response fields, from `pulumi/pulumi`
  `pkg/backend/httpstate/client/client.go` and `sdk/go/common/apitype`.
  `/api/user` has `tokenInfo` only for team and org tokens. Decrypt is a POST.
- `internal/populate/command.go` spools raw stdout to a private temp file
  before the transform, which is why the Pulumi source stays in memory.

## Follow-ups filed

`pudl-7g8j.1` to `pudl-7g8j.8` (see the design's Delivery table).
