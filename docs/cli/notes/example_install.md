Available example: `git-inventory`. It includes the model, saved observations,
and optional live observer; no checkout or download is needed. Identical files
are left alone. Conflicting edits or symlinked destinations cause an error before
installation writes files. This command does not initialize global state.

`--json` returns `example`, the workspace `root`, and relative `files` paths.
See [getting started](getting-started.md) for the PUDL-only catalog replay
tutorial. Live observation additionally requires mu and Python 3.
