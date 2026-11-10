# Live check: Pulumi org token with a read-only role (2026-11-10)

Issue: `pudl-7g8j.10` (closed). Design: `docs/design/2026-11-06-pulumi-pack.md`
§3, "Live check".

## Method

- GET requests only, to `https://api.pulumi.com`, with redirects refused.
- A probe script outside the repo printed response shapes with every string
  value redacted. It printed only descriptor `__type` values, scope names,
  the secrets provider type and IDs as list labels (`<set#2>`). The token
  and org values were never printed or stored.
- The export check walked each state in memory and printed only key names
  and envelope key sets.

## Findings

- `/api/user` for an org token: `tokenInfo{name, organization, team: null}`.
- `roles?uxPurpose=role&assignedToMe=true` returns exactly one role. Its
  `details` can be a `PermissionDescriptorAllow` with inline scopes.
- The token lists policies and sets with no `role:read` scope. Policies use
  `Group`, `Compose` and `Condition` descriptors; sets carry `resourceType`.
- Some read scopes do not end in `:read` (`environment:list_deleted`,
  `resources:search`), so the guard needs an explicit allow list.
- The stack list works with no `stack:list` scope. `stack:export` is
  sufficient for the export.
- The export has secret envelopes with only the signature and
  `ciphertext`, and no `plaintext`. The secrets provider is `service`.

## Output

- Fixtures with placeholder values only, in
  `packs/pulumi/populators/pulumi/`: `guard-user-org-token.json`,
  `guard-roles-assigned.json`, `guard-roles-policies.json`,
  `guard-roles-sets.json` (one safe set and one set with `stack:decrypt`),
  `stacks.json` and `export.json`.
- Design §3 updated: guard condition 1 also requires `team: null`;
  condition 3 accepts a role with inline scopes and uses an explicit
  allow list. Open question 1 answered.
- No public API change. Unblocks `pudl-7g8j.3`.
