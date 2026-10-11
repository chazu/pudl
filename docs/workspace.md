# Workspace schema resolution

A repository workspace resolves schemas, models, rules, and definitions from
its own files and explicitly declared vendored dependencies. It does not inherit
ambient `~/.pudl` definitions. `pudl --global` explicitly selects personal state,
and outside a repository workspace the personal workspace remains the default.

Schema/model precedence is project first, then dependencies in declared order.
Rule precedence is equivalent; project rules also override a model's adjacent
rules directory. The CLI and `pkg/factstore.DiscoverWorkspace` use one policy.
`pudl config --paths --json` exposes every effective path, and
`pudl schema list --json` / `pudl import --explain` expose schema provenance.

## Shared definition packages

Vendor each package under `.pudl/vendor/<name>/` with its own `schema/` tree and
CUE module, plus any `definitions/` and `populators/` the package requires.
Declare the package roots in `.pudl/workspace.cue`:

```cue
name: "my-project"
dependencies: ["vendor/team"]
```

Commit the vendored files and the declaration together; the Git revision pins
their contents. CUE module dependencies inside those files retain their native
version declarations. Package paths are relative and must remain under
`.pudl/vendor`; missing packages, duplicates, and symlinks are rejected. Portable
workspace bundles include vendored packages. PUDL does not fetch these package
directories. Their CUE imports retain native module-resolution behavior; prepare
those module dependencies separately when offline operation is required.

To migrate a workspace that relied on global definitions:

1. Run `pudl config --legacy-dependencies --json` to inventory ignored global
   schema/rule names and their source root. This only inspects them.
2. Copy the needed CUE packages, required CUE module files, rules and populator
   assets into a chosen vendored package. Review that set; PUDL does not copy
   unrelated home-directory configuration automatically.
3. Declare it in `workspace.cue`, inspect `pudl config --paths --json`, and run
   model validation and representative checks under a clean home directory.
4. Commit the vendored content. Keep the global originals for personal-mode use.

For a small customization, copying the required definitions directly into the
project's existing schema tree is sufficient; no dependency package is required.
Built-in schemas and rules are installed locally by `pudl init`.

## Local persistence

Workspace-local schemas and model files are project-owned and should be
committed with the repository. Mutable runtime state is isolated beneath
`<repo>/.pudl/data/`: raw imports, metadata, SQLite catalog rows, facts,
snapshots, run reports, and approval records never share the global catalog.
The generated `.pudl/.gitignore` keeps runtime data and the machine-local path
configuration out of Git.
Repository `schema_path` and `data_path` are fixed to `.pudl/schema` and
`.pudl/data`; PUDL rejects configuration that would redirect mutable state
outside the workspace boundary.

`pudl init` is safe to repeat. It creates or repairs the local data layout,
CUE module, every built-in schema, `pudl/systemmodel.#SystemModel`, and
`.pudl/schema/models/` (the path used by `pudl model new`) while preserving an
authored `workspace.cue` unless `--force` is requested. Outside a repository
workspace, the equivalent state lives beneath `~/.pudl/`.
