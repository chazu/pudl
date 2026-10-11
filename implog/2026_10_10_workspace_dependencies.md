# Reproducible project definitions

Repository definition search uses project-local files and `dependencies` listed
in workspace.cue. Packages are vendored under .pudl/vendor and versioned with the
project; missing, escaping and symlinked dependencies fail clearly. Bundles
include vendor content. `--global` explicitly selects personal state, while
`config --paths` shows effective paths and `config --legacy-dependencies`
inventories global migration candidates without copying or activating them.
CLI and library resolution share workspace.Policy.

Validation: workspace, public factstore, bundle and cmd tests passed. Added
conflicting/clean home, vendored precedence, confinement and resolution-parity
checks. Existing user definitions and global data are not migrated automatically.
