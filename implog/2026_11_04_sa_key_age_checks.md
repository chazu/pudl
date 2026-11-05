# Service account key checks, and datalog date comparisons with "now"

**Date:** 2026-11-04

## Work done

### Product: `"<now-90d"` comparisons in rule bodies

A body argument `"OP now"` or `"OP now-N(d|h|m)"` (OP one of `<`, `<=`,
`>`, `>=`) compares a date string with the evaluation time minus N days,
hours or minutes. The evaluation time is the query's valid time (`--as-of`)
when set, else the current time. Only text values compare (SQLite's
julianday would read a number as a Julian day); a string that is not a date
matches nothing. Body-only, like numeric comparisons.

- `internal/datalog/types.go`: `Term.Ago`, `nowTermPattern`, parsing.
- `internal/datalog/compile.go`: `(typeof(x) = 'text' AND julianday(x) OP
  julianday(?, 'unixepoch'))`; `evaluationTime(scope)`. No query caching, so
  each evaluation computes its own "now".
- `internal/datalog/now_comparison_test.go`: operators and units, time
  travel, non-dates and numbers, parsing, head rejection.
- `docs/datalog.md`: comparisons (numeric ones were undocumented too).

### GCP pack: `gcp[-fixture]-iam-hygiene-checks`

| Check | Query | Severity |
|---|---|---|
| `sa-key-older-than-90d` | `gcp_sa_key_older_than_90d(key, account)`: enabled user-managed key with `valid_after < now-90d` (CIS GCP 1.7) | fail |
| `sa-key-never-expires` | `gcp_sa_key_never_expires(key, account)`: enabled key with `validBeforeTime` 9999-12-31T23:59:59Z | warn |

Collector: `service-account-keys`. Rules in
`packs/gcp/schema/pudl/rules/gcp_pack.cue`, family in `gcp_pack_checks.cue`.
Fixture result: legacy-ci's 2022 never-expiring key fails both; the
disabled ops key fails neither. Tests: `cmd/gcp_pack_test.go` (family list,
findings, remediation stage drops the key).

The 90-day threshold is fixed in the rule; rules and models are different
CUE packages, so a models setting cannot reach it.

## Public API

No Go API change. Datalog surface: `"OP now[-N(d|h|m)]"` body terms. Pack
surface: models `gcp-iam-hygiene-checks`, `gcp-fixture-iam-hygiene-checks`;
relations `gcp_sa_key_older_than_90d`, `gcp_sa_key_never_expires`.
