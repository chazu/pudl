## Output contract

Use `--help` for inline documentation. Under the global `--json` flag, a
command writes exactly one JSON document to stdout; warnings, progress and
other diagnostics go to stderr, and empty lists are written as `[]`. Commands
without structured output (scaffolding, editors, Git and CUE wrappers) still
print text under `--json`.

Errors exit nonzero. `pudl run` and `pudl run set` can also report their
findings in the exit status with `--detailed-exitcode`; see
[pudl run](#pudl-run).
