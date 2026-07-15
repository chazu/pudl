# Adversarial review closure

Completed the implementation tickets created from the repository adversarial
review and aligned the current documentation with the resulting behavior.

## Implemented behavior

- `pudl run --converge --only` accepts exact resource selectors, rejects unknown
  selectors before side effects, and includes declared resource dependencies.
- Normal inventory runs populate and compare their current observe snapshot;
  `--from-catalog` is the explicit catalog replay path.
- Content hashing uses readers, collection imports are atomic, and shared items
  use normalized many-to-many collection memberships with safe cascade deletion.
- CLI envelope extraction is unified across regular, batch, and stdin imports.
- Workspace schema resolution is local-first with global fallback, including
  shadowing behavior for schema commands and model/run resolution.
- CI targets Go 1.25.8 and runs build, test, vet, generated-skill, race, and
  manually-triggered smoke gates.

## Public surfaces

The current contracts are documented in `docs/cli-reference.md`,
`docs/collections.md`, `docs/workspace.md`, `docs/TESTING.md`, and the generated
`pudl-core` skill. Beads Rust issue state is tracked in `.beads/`.
