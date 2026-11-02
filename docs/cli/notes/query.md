In a project, rules come from `.pudl/schema/pudl/rules/` and the rules of
vendored dependencies; in global mode, from `~/.pudl/schema/pudl/rules/`. A
query fails when the queried relation depends on a rule that cannot be loaded;
`pudl doctor` lists such rules. See [Datalog](datalog.md) for rules,
built-in relations and time travel.

Every positional constraint must be `field=value` with a nonblank field;
repeated fields are errors. Empty values and values containing `=` are valid.
`--list` takes no positional arguments and cannot be combined with `--topo`.
`--max-iterations` must be nonnegative; zero uses the default.
`--timeout` bounds evaluation and waiting for writers; a cancelled query
returns an error, never a partial answer. If stdout fails mid-write, a
consumer may receive a prefix, so automation must check the exit status.
