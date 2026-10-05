Initialization installs configuration, a local CUE module and built-in schemas,
authoring and data directories, and bundled Claude skills. Repeated
initialization preserves authored configuration; `--force` replaces
configuration while preserving data.

`--json` returns the workspace `path` and `mode`.

`--from-bundle FILE` restores verified history into a new local workspace. Existing state is never overwritten. Pending approvals require replanning and restored snapshots cannot authorize producer bindings. See [backup and restore](evidence.md#backup-and-restore).
