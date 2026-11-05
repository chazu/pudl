# populate.fail_when: warnings that hide a failure (pudl-zbj)

Date: 2026-11-05

## Problem

gcloud's aggregated list commands (compute lists that span regions or zones:
addresses, subnets, routers, instances, backend services, forwarding rules
and others) exit 0 when a project refuses the list. They print
`WARNING: Some requests did not succeed.` and the missing permission on
stderr, and `[]` (or what they could read) on stdout. pudl took the run at
its word: the project was observed with no resources, the observation stayed
complete, and every resource seen there before was marked gone. Found while
verifying batch (b) of pudl-q4v.7 live; it affected existing collectors,
including `gcp-instances` (gcloud only) and every CAI collector's fallback.

## Change

A command source (a populate or its fallback) can declare `fail_when`:
regular expressions (Go RE2) matched against the stderr of a run that exited
0. A match makes the run a failed input with exit status 0: its records are
discarded, the snapshot is incomplete, and the error names the pattern and
the stderr line. It is never an empty input and it trips `fail_fast`. It
does not apply to the `each` command.

### Public API

- CUE `#CommandSource.fail_when?: [...string & !=""]`
  (`internal/systemmodel/schema.cue`).
- `systemmodel.Populate.FailWhen []string` (`json:"fail_when"`) and
  `Populate.FailPatterns() ([]*regexp.Regexp, error)`; validated on load,
  also for `fallback.observe`. `EmptyPatterns` and `FailPatterns` share one
  compiler (`internal/systemmodel/stderr_patterns.go`, renamed from
  `empty_when.go`).
- `internal/populate`: `execute` returns the stderr tail of a successful run
  too; `attempt` takes the fail patterns; `matchFail` (`fail_when.go`)
  builds the failed input.

### GCP pack

`_gcpFailWhen: ["Some requests did not succeed"]` (`gcp_pack.cue`) is the
`fail_when` of every live collector's gcloud source.

## Verification

- `internal/populate/fail_when_test.go`: a matching run that exited 0 is a
  failed input (exit 0, records dropped, never empty); fail_fast trips; a bad
  pattern stops the populate.
- `internal/systemmodel/stderr_patterns_test.go`: loads `fail_when`; bad and
  empty patterns fail the load, in a fallback too.
- `cmd/gcp_pack_live_test.go`: the fake gcloud's `proj-hidden` warns and
  exits 0 on compute lists; `gcp-addresses` (through its fallback) and
  `gcp-instances` report it as a failed input and an incomplete snapshot.
- Live (scratch workspace, nothing kept): `gcp-addresses` through gcloud now
  reports the projects that refuse the list as failed inputs with exit
  status 0, instead of empty projects.
- `CGO_ENABLED=0 go test ./...` passes.

Docs: `docs/observation-scope.md` ("Warnings that hide a failure"), the
pudl-core skill, `packs/gcp/README.md`.
