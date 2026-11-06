# #HTTPObserve: guard with several named requests (2026-11-06)

Issue: `pudl-7g8j.9` (epic `pudl-7g8j`). Design:
`docs/design/2026-11-06-pulumi-pack.md` §3.

## Problem

The Pulumi token guard must read a token's identity, its role, the role's
policy and the permission sets the policy reaches. These are separate GET
responses, and a single-request guard cannot check one against another.

## What changed

1. **Schema:** `guard.get` is `#HTTPPath | {[string]: #HTTPPath}`.
2. **Loader** (`internal/systemmodel/http_guard.go`): `HTTPGuard.Requests`
   holds the paths by name; a single path is the one request named "".
   An empty map and an empty name are rejected. `MarshalJSON` records
   `get` as declared, which still satisfies `#SystemModel`.
3. **Executor** (`internal/populate/http.go`): `httpSource.guard` sends the
   requests in name order. With one path, `check.in` is its response; with
   named paths, `check.in.<name>` is each response. A failed guard request
   or a check that does not pass fails the populate, and nothing else is
   sent.

## Public surface

- CUE: `guard: {get: "/path" | {name: "/path", ...}, check: {in: _, ok,
  reason?}}`.
- Go: `systemmodel.HTTPGuard{Requests map[string]string, Check}` with
  `Single()`, `Names()`, `MarshalJSON()`. `HTTPGuard.Get` was removed.

## Tests

`TestLoadModelReadsANamedGuard`, `TestLoadModelRejectsBadNamedGuards`
(systemmodel); `TestHTTPRunChecksNamedGuardRequestsTogether` (populate):
name order, a check that joins three responses, refusal, and a failed
guard request.
