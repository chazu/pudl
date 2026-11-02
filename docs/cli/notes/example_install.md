Available examples: `git-inventory` and `gcp-network-hygiene`. Each includes
its model and fixture data; no checkout or download is needed. Identical files
are left alone. Conflicting edits or symlinked destinations cause an error before
installation writes files. This command does not initialize global state.

`--json` returns `example`, the workspace `root`, and relative `files` paths.
See [getting started](getting-started.md) for the Git inventory tutorial; it
needs only PUDL, Git, `cat` and `cp`.
