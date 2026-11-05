# Workspaces

A workspace holds everything pudl knows about one project. Run `pudl init` in a
repository to create one in `.pudl/`. pudl finds it by walking up from the
current directory. Outside a repository workspace, or with `--global`, pudl
uses the personal workspace in `~/.pudl/`. The two never share a catalog.

## Layout

| Path | Contents | In Git |
| --- | --- | --- |
| `.pudl/workspace.cue` | Workspace name and declared dependencies | yes |
| `.pudl/schema/` | CUE module: types, policies, models (`models/`) and rules (`pudl/rules/`) | yes |
| `.pudl/populators/` | Fixtures and helper files that models read | yes |
| `.pudl/definitions/` | Desired-state definitions | yes |
| `.pudl/vendor/` | Vendored definition packages (see below) | yes |
| `.pudl/data/` | Raw records, catalog, facts, snapshots and reports | no |
| `.pudl/config.yaml` | Machine-local configuration | no |

The generated `.pudl/.gitignore` keeps `data/` and `config.yaml` out of Git.
`data/` is readable by its owner only; `pudl doctor` reports looser
permissions and `pudl doctor --fix --only data-permissions` repairs them.
In a repository workspace, the schema and data paths are fixed to
`.pudl/schema` and `.pudl/data`; pudl rejects configuration that points state
outside the workspace.

`pudl init` is safe to repeat. It creates or repairs the data layout, the CUE
module and the built-in schemas (including `pudl/systemmodel.#SystemModel`).
It also installs the pudl agent skill in `.claude/skills/`. It keeps an edited
`workspace.cue` unless you pass `--force`. Run it again after upgrading pudl
to refresh the agent skill. The built-in schemas also refresh on their own:
the first command a new pudl runs in the workspace rewrites them (pudl owns
these files, so edits to them are lost), removes retired ones it shipped, and
says so on stderr. This holds in global mode too (`pudl --global check`,
`pudl --global model validate`), and `pudl init --global` repairs an existing
`~/.pudl` the same way, keeping its `config.yaml` unless you pass `--force`.

`pudl config --paths --json` prints every effective path.

## Where definitions come from

A repository workspace reads schemas, models and rules from its own
`.pudl/schema/` and from the packages it declares. It does not read
definitions from `~/.pudl`. When names collide, the project wins, then
dependencies in declared order. Project rules also override rules that sit
next to a model. The CLI and `pkg/factstore.DiscoverWorkspace` follow the same
order. `pudl schema list --json` and `pudl import --explain` show where each
schema came from.

## Shared definition packages

To share schemas, models and rules between repositories, vendor each package
under `.pudl/vendor/NAME/`, with its own `schema/` tree and CUE module plus
any `definitions/` and `populators/` it needs. Declare it in
`.pudl/workspace.cue`:

```cue
name: "my-project"
dependencies: ["vendor/team"]
```

Commit the vendored files and the declaration together; the Git revision pins
their contents. Package paths are relative and must stay under `.pudl/vendor`.
pudl rejects missing packages, duplicates and symlinks. pudl does not download
packages. CUE imports inside them resolve as usual, so fetch those module
dependencies yourself if you need to work offline.

For a small change, copy the definitions you need into `.pudl/schema/`
instead; no package is required.

## Moving from global definitions

A repository workspace ignores definitions in `~/.pudl`. To find the ones a
project relied on:

1. Run `pudl config --legacy-dependencies --json`. It lists the global schemas
   and rules the workspace ignores, and changes nothing.
2. Copy the CUE packages, module files, rules and populator files you need into
   `.pudl/schema/` or a vendored package.
3. Declare any package in `workspace.cue`, then run `pudl config --paths --json`,
   `pudl model validate NAME` and a representative `pudl check`.
4. Commit the result. The global originals stay available in global mode.

## Backups and moving a workspace

`pudl export --bundle FILE` writes a verified, portable bundle of the
workspace's history, including vendored packages.
`pudl init --from-bundle FILE` restores it into a new workspace. See
[evidence and recovery](evidence.md).
