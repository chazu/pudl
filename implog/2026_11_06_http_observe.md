# #HTTPObserve: a GET-only HTTP populate source (2026-11-06)

Issue: `pudl-7g8j.1` (epic `pudl-7g8j`). Design:
`docs/design/2026-11-06-pulumi-pack.md` §1.

## Problem

Collectors could only run commands. Reading an HTTP API meant a shell helper
and `curl`/`jq`, with the raw response spooled to a temp file before the CUE
transform, and nothing to stop a write request or a token sent elsewhere.

## What changed

1. **Schema** (`internal/systemmodel/schema.cue`): `#HTTPObserve` joins the
   populate disjunction, with `#HTTPPath` and `#HTTPRun`. The arms stay
   unambiguous: command runs need `argv`, HTTP runs need `get`, and both
   definitions are closed.
2. **Model types** (`internal/systemmodel/http_spec.go`, `predicate.go`):
   `Populate.HTTP *HTTPObserve` and `KindHTTP`. The loader reads the guard
   check, the item mapping and the run transforms from CUE (they depend on
   `in`), and validates paths (relative, same origin), the item placeholder,
   and headers (no `Host`, no copy of the auth header). `Populate.MarshalJSON`
   records the HTTP shape, which still satisfies `#SystemModel`.
3. **Executor** (`internal/populate/http.go`, `http_records.go`): `Run`
   dispatches to `runHTTP`. GET only; every URL is checked against `base`;
   redirects to another origin are refused; responses are read under the
   staging byte budget and decoded in memory with exact numbers.
4. **Failure handling.** A transport error, non-2xx, non-JSON body, missing
   records path or failed transform is a failed input (`FailedInput.Request`,
   no body kept). A missing token, a guard that does not pass, a failed
   enumeration, a rejected key, the byte budget and the context ending fail
   the populate. Transform, item and guard errors name CUE paths only
   (`systemmodel.ErrorPaths`), because CUE messages quote values.
5. **Reports and CLI.** Failed HTTP inputs render as `item: error` with a
   `request:` line; `pudl model show` prints the base and requests;
   `pudl model validate` reports an unset `auth.env` variable.
6. **Ratchet** `no-outbound-http-outside-populate` (budget 0): no
   `http.Get`/`Post`/`NewRequest*`/`DefaultClient` or `http.Client`/
   `Transport` outside `internal/populate` and tests. A `[...]` alternation
   in the query matched nothing, so the rule uses two patterns.

## Public surface

```cue
#HTTPObserve: {
	schema?, allow_schema_fallback?, timeout?, fail_fast?
	base:     =~"^https://[^/?#@]+$"
	headers?: {[string]: string}
	auth?:    {env, header: *"Authorization", scheme: *"Bearer"}
	guard?:   {get: #HTTPPath, check: {in: _, ok: bool, reason?: string}}
	each?:    {get, list, item?: {in: _, out: string}, page?: {next, param}}
	runs:     [...{get, records?, set?, transform?}]
	reject_keys?: [...string]
}
```

Go: `systemmodel.HTTPObserve`, `HTTPAuth`, `HTTPGuard`, `HTTPEnumeration`,
`HTTPPage`, `HTTPRun` (`ForItem`), `HTTPObserve.URL`, `ValidateItem`,
`Predicate` (`NewPredicate`, `Eval`), `Transform.String`, `ErrorPaths`,
`KindHTTP`; `populate.Request.HTTPClient` and `Getenv`;
`populate.FailedInput.Request`.

## Differences from the design

- `headers` was added for static, non-secret headers (an API's `Accept`).
- `guard.check` and `each.item` are optional in the schema because they are
  not recorded with the model instance; the loader requires `check`.
- Enumerated inputs also come from string elements when `item` is absent.

## Tests

`internal/populate/http_test.go` (TLS test server): guard, paging with
dedup, item mapping, records path, transform, set, GET-only with the token;
guard refusal and unevaluable checks; missing token; failed inputs without
bodies; `fail_fast`; rejected keys and value-free transform errors;
cross-origin redirect; inputs that would leave the path; staging limit.
`internal/systemmodel/http_spec_test.go`: loading, recording against the
schema, rejected shapes, item validation, `ErrorPaths`.
`internal/checkreport/populate_test.go`: report rendering.
