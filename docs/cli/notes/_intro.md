## Output contract

Use `--help` for inline documentation. Under the global `--json` flag, a
command writes exactly one JSON document to stdout; warnings, progress and
other diagnostics go to stderr, and empty lists are written as `[]`. A few
commands without structured output, such as scaffolding, still print text
under `--json`.

Errors exit nonzero. `pudl check` can also report its findings in the exit
status with `--detailed-exitcode`; see [pudl check](#pudl-check).
