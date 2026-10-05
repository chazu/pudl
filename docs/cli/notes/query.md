Rules are loaded from `.pudl/schema/pudl/rules/` (repo-scoped) and
`~/.pudl/schema/pudl/rules/` (global). A query fails when the queried relation
depends on a rule that cannot be loaded; `pudl doctor` lists such rules. See
[datalog](datalog.md) for evaluation and rule authoring.

`--timeout` bounds SQL evaluation, recursion, and writer contention independently of Mu subprocess timeouts. Cancellation returns an error and never a successful partial answer.
