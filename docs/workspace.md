# Workspace schema resolution

When `pudl` runs inside a repository workspace, it searches schemas in this
order:

1. `<repo>/.pudl/schema/`
2. `~/.pudl/schema/`

The first matching schema definition wins. A workspace-local schema can
therefore override a global definition without copying the rest of the global
schema repository. Outside a workspace, only the global schema path is used.

The same ordered paths are used by import inference, validation, observe
ingestion, schema list/show/completion, and model/run resolution. `pudl config`
prints both the configured global path and the effective search order.

Workspace-local schemas and model files are project-owned and should be
committed with the repository. Mutable runtime state is isolated beneath
`<repo>/.pudl/data/`: raw imports, metadata, SQLite catalog rows, facts,
snapshots, run reports, and approval records never share the global catalog.
The generated `.pudl/.gitignore` keeps runtime data and the machine-local path
configuration out of Git.
Repository `schema_path` and `data_path` are fixed to `.pudl/schema` and
`.pudl/data`; PUDL rejects configuration that would redirect mutable state
outside the workspace boundary.

`pudl repo init` is safe to repeat. It creates or repairs the local data layout,
CUE module, every built-in schema, `pudl/systemmodel.#SystemModel`, and
`.pudl/schema/models/` (the path used by `pudl model new`) while preserving an
authored `workspace.cue` unless `--force` is requested. Outside a repository
workspace, the equivalent state lives beneath `~/.pudl/`.
