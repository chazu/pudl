Exports honor source formats and collection membership. Read/serialization failures
fail by default; `--allow-partial` publishes readable entries with a nonzero exit
and explicit diagnostics. File destinations are replaced atomically after success.
CSV columns are sorted and support scalar object fields.

`--bundle FILE` captures the complete local workspace, without entry filters.
See [evidence and recovery](evidence.md) for portable bundles, limits, and restore.
