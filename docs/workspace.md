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
committed with the repository. The catalog and imported data remain under the
global PUDL data directory unless a future workspace data policy says
otherwise.
