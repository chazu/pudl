# Scope reduction completion and qualification

All eight implementation steps in
[the delivered plan](../docs/design/2026-10-10-scope-reduction-and-improvement-order.md)
are complete. `list --fancy` and its dependencies remain supported at the user's
request. Existing authored data and history are preserved; compatibility changes
and native tool replacements are documented in
[retired commands](../docs/retired-commands.md) and
[workspace resolution](../docs/workspace.md).

## Delivered behavior

- Retired shell-profile management, guessed dependency derivation, and
  Git/editor/CUE wrappers. Kept completion and the interactive list.
- Explicit schemas validate before publication, with a visible permissive opt-in.
- Checks distinguish pass, fail, unknown, and error, and retain their diagnostics.
- Explicit observation scopes, completeness and freshness support checks over
  selected snapshots without mixing unrelated latest-known facts.
- Nested/composite identity and routing-tag matching use the same identity family
  as imported records. Unobserved resources in partial inventories remain uncertain.
- Vendored project dependencies replace ambient global definition inheritance.
- Standalone and aggregate reports expose execution, conformity, checks,
  verification, scope, evidence, and next actions. Early JSON failures are structured.
- A CLI-independent check service, progress events, payload inspection, saved
  checks, snapshot comparison, resource history and repair suggestions support
  the bundled GCP fixture and separately named live collector.

## Acceptance evidence

The final code qualification used source `cd1583e`:

| Check | Result |
| --- | --- |
| `mise exec -- make test-race` | Passed all packages with race/checkptr enabled; Datalog completed in 288.606s |
| `mise exec -- make lint` | Passed, zero issues |
| `mise exec -- make check-skills check-docs` | Passed |
| `mise exec -- make build` | Passed; built binary reported commit `cd1583e` |
| `mise exec -- make test-git-walkthrough` | Passed installed-example and tutorial acceptance |
| `mise exec -- make test-kick-tires` | Passed real-Mu observation, exact approvals, stale-plan rejection, sealed routing, fail-fast, and concurrent run sets |
| Native consolidated-command smoke | Passed; retired paths fail and current paths work |
| GCP fixture workflow | Passed discovery, evidence inspection, saved checks, correction, snapshot comparison, history and original-report retention |
| Interactive list | Native PTY list rendered and exited successfully with `q` |
| Documentation links and Git whitespace checks | Passed |

The individual implementation logs record focused regressions and the scripted
baseline comparison. The full race gate includes the final identity and
incomplete-projection fixes. This is local qualification; no CI result or
release publication is claimed.

## Remaining qualification and follow-up

`mise exec -- make vulncheck` did not pass: govulncheck v1.8.0 reports 12 reachable
standard-library advisories against the existing Go 1.26.6 pin, with related
network-dependency findings. `pudl-ew9` tracks upgrading the Go and dependency
pins and re-running the scanner. The toolchain/dependency upgrade is separate
from this scope-reduction implementation; no clean security scan is claimed.

Subsequent remediation: `pudl-ew9` upgrades the toolchain and dependencies, and
the fresh scanner reports no vulnerabilities. See the
[remediation log](2026_10_10_vulnerability_remediation.md) for versions and
acceptance evidence; the failed scan above records the original qualification.

The GCP collector was qualified through local fixtures, not a credentialed live
project. An actual human usability trial was not performed. The walkthrough is
runnable and covered by agent-driven CLI tests; the trial timing excludes human
authoring and reasoning time. No installed-binary update was requested or made.
