Rules are loaded from `.pudl/schema/pudl/rules/` (repo-scoped) and
`~/.pudl/schema/pudl/rules/` (global). A query fails when the queried relation
depends on a rule that cannot be loaded; `pudl doctor` lists such rules. See
[datalog](datalog.md) for evaluation and rule authoring.

`--timeout` bounds SQL evaluation, recursion, and writer contention independently of Mu subprocess timeouts. Cancellation returns an error and never a successful partial answer.
Every positional constraint must be `field=value` with a nonblank field; repeated
fields are errors. Empty values and values containing `=` remain valid.
`--list` accepts no positional arguments and cannot be combined with `--topo`.
`--max-iterations` must be nonnegative; zero uses the default recursion cap.
Every query presentation mode returns output errors. A failed stdout consumer may
receive a prefix; automation must check the command's exit status.
