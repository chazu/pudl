# Evidence verdicts and empty observations

Command observers now retain empty arrays as zero-record snapshots. Check
results distinguish pass, fail, unknown, and evaluation error. Dependency-scoped
projection diagnostics prevent absent relations and broken required projections
from producing passing checks; unrelated failures do not disable a check.
Uncertain checks persist their diagnostics, exit 1 with detailed status, and
cannot promote model resources to verified clean. Reports now use version 2;
version-1 stored reports remain readable.

Validation: full cmd, projection and acute package tests; focused missing-evidence,
empty-output, broken-projection, sync-failure and unrelated-schema regressions.
