Available examples: `git-inventory`, `gcp-network-hygiene` and `gcp`. Each
includes its model and fixture data; no checkout or download is needed.
Identical files are left alone. Conflicting edits or symlinked destinations
cause an error before installation writes files. This command does not
initialize global state.

`gcp` is a content pack: draft schemas for the core GCP inventory,
best-practice policies, gcloud and fixture collectors, and three check
families. See [the pack README](../examples/gcp/README.md).

`--json` returns `example`, the workspace `root`, and relative `files` paths.
See [getting started](getting-started.md) for the Git inventory tutorial; it
needs only PUDL, Git, `cat` and `cp`.
