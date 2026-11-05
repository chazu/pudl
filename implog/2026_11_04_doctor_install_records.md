# Doctor accepts install records (pudl-bpy)

## Problem

`pudl example install` records what it wrote in `<workspace>/.examples`.
`pudl doctor`'s directory-structure check then warned
`Unexpected top-level entries ... [.examples]`, although the directory is
pudl's own bookkeeping. `pudl pack install` (new today) records in `.packs`
and would have hit the same warning.

## Root cause

`CheckDirectoryStructureAt` (`internal/doctor/checks.go`) validates the
workspace root against a closed list of allowed entries, which predates
install records.

## What changed

- `internal/doctor/checks.go`: the allowed list includes
  `examples.Kind.RecordDir` (`.examples`) and `packs.Kind.RecordDir`
  (`.packs`), taken from the registries so the names have one source.

## Public API

None.

## Tests

- `internal/doctor/structure_test.go`: a workspace with `.examples` and
  `.packs` passes the check; a stray entry is still reported, alone.
