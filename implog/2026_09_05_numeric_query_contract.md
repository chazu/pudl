# Numeric query values and equality

**Date:** 2026-09-05

**Tracker:** `pudl-wby`

## Outcome

The adjacent integers `9007199254740992` and `9007199254740993` now remain
distinct through base, derived, recursive, current, and historical queries.
An exact constraint on the second value returns one tuple containing that
value, including CLI JSON output.

The underlying causes were independent losses at multiple boundaries:

- Base fact JSON decoded into float64, and SQL result scanning converted
  INTEGER values to float64.
- Historical Go filtering compared numeric operands after float64 coercion.
- SQLite JSON extraction could round numeric tokens before comparison or
  deduplication, while CUE numeric extraction ignored range/conversion errors.

The database now owns a shared checked numeric conversion, reusing the exact
decimal canonicalization from fact identity. A registered SQLite scalar
function checks JSON tokens obtained with `->` before SQL filters, joins,
aggregation, and recursive table keys use them. Current-fact filtering and
the scored-fact view use the same boundary. Base reads normalize nested JSON
numbers, and historical comparisons preserve exact integer/binary64 values.
Numeric rule terms retain their JSON literals until compilation checks them.

## Public API and compatibility

No public functions or types were added. `Store.Query` now returns integral
fact values and SQLite INTEGER results, including counts, as `int64`.
Fractions and SQLite REAL results remain `float64`. Callers that asserted
every numeric result was float64 must handle int64 too.

The query input domain is signed int64 integers plus non-integral decimals
whose values survive float64 conversion and Go JSON serialization unchanged.
Out-of-range integers, precision-losing decimals, overflow, underflow, and
non-finite numeric operands return explicit errors. A Go float operand uses
its actual binary value; an exact int64 or json.Number operand is preferred
when starting with decimal evidence. CUE-loaded numeric `Term.Value` fields
now contain json.Number. Equivalent integer and exponent spellings compare
equally. Existing REAL arithmetic remains SQLite binary64 arithmetic.

CLI numeric constraints now parse numerically. JSON quotes request a string,
for example `pudl query numbers 'n="9007199254740993"' --json`.

Stored facts, IDs, and raw QueryFacts/FactHistory results are unchanged.
Unsupported query numbers remain available as raw evidence. Direct SQLite
clients reading the scored-fact view need the registered `pudl_query_value`
function; PUDL registers it before opening normal or read-only catalogs.

## Validation

The original public API reproducer failed again before editing. New public
regressions failed twice before the fix, covering result representation,
equivalent spellings, and unsupported numeric values.

Passing regression coverage includes all four temporal scopes; signed int64
boundaries; adjacent positive and negative integers above 2^53; nonlinear
numeric closure; joins, CUE ground terms and comparisons; decimals, nested
values, extreme exponents, invalid operands; and real CLI handler JSON output.
The original reproducer now reports distinct values and one exact match in
both current and historical base/derived queries.

Passed under `mise exec`:

- `go test ./...`
- `go test -race -gcflags=all=-d=checkptr=0 ./...` (repository CI exception)
- `go vet ./...`
- `go build ./...`
- `go run ./internal/skills/gen -check`

`br lint` passed for all three new tickets. Real-mu smoke execution is outside
this local query-only validation; the repository CI workflow also runs it.

## Follow-up dependencies

- `pudl-sjk`: maintained Git inventory and drift walkthrough, depends on
  `pudl-wby`.
- `pudl-qrl`: explainable human and JSON reports, depends on `pudl-sjk`.

Both follow-ups remain open. Only numeric query correctness was implemented.
