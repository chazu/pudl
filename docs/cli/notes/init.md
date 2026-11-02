Initialization installs configuration, a local CUE module and built-in schemas,
authoring and data directories, and bundled Claude skills. Repeated
initialization preserves authored configuration; `--force` replaces
configuration while preserving data.

`--json` returns the workspace `path` and `mode`.

`--from-bundle FILE` restores a verified bundle into a new workspace and never
overwrites existing state. Restored snapshots do not count as a model's latest
observation until the model is observed again. See
[backup and restore](evidence.md#backup-and-restore).
