Rules are loaded from `.pudl/schema/pudl/rules/` (repo-scoped) and
`~/.pudl/schema/pudl/rules/` (global). A query fails when the queried relation
depends on a rule that cannot be loaded; `pudl doctor` lists such rules. See
[datalog](datalog.md) for evaluation and rule authoring.
