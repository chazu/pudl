# Spike: a read-only Pulumi Cloud token pudl can verify (2026-11-06)

Issue: `pudl-7g8j.2` (closed). Design: `docs/design/2026-11-06-pulumi-pack.md`
§3, rewritten with the findings.

## Questions

1. Does Pulumi Cloud offer a role that can read and export stacks without
   decrypting secrets or writing?
2. Can a token find out its own role and scopes with GET requests only?

## Findings

- Custom roles (Pro and Enterprise) are built from permission sets of
  scopes. `stack:read`, `stack:list`, `stack:export` are separate from
  `stack:decrypt`, `stack:encrypt`, `stack:write` and the other stack
  scopes, so a read-only set is possible.
- The built-in "Stack Read" permission set includes `stack:decrypt` and
  `stack:encrypt`; it is not safe for pudl.
- Org access tokens take exactly one role; team tokens the union of their
  team's roles; personal tokens all of the user's permissions.
- `GET /api/user` returns `tokenInfo{name, organization, team}` and no role.
- `GET /api/orgs/O/roles?uxPurpose=role&assignedToMe=true` returns the
  caller's roles. A role's `details` is a descriptor tree (`Compose`,
  `Group`, `Condition`) referring to a policy; `uxPurpose=policy` and
  `uxPurpose=set` list policies and permission sets (sets carry
  `permissions`). Without `uxPurpose` the list is HTTP 400;
  `GET /api/orgs/O/roles/{id}` needs more rights than the list.
- Sources: the public OpenAPI spec (`/api/openapi/pulumi-spec.json`), the
  Pulumi docs on roles and access tokens, and GET calls with `pulumi api`.

## Outcome

- Guard design: org token only, pinned by organization and name, and one
  CUE check over the role, policy and set lists that allows only an allow
  list of read scopes and fails on any unknown descriptor shape.
- Follow-ups: `pudl-7g8j.9` (guard with several named requests),
  `pudl-7g8j.10` (live check with an org token; needs an org admin). Both
  block `pudl-7g8j.3`.
