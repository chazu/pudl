# User schema namespace convention

Date: 2026-06-10

Documents the recommended namespace convention for user-authored schemas, plus
a `pudl doctor` check that enforces it.

## Background

PUDL does not enforce any package path for user-defined CUE schemas — the
package path is just the directory a `.cue` file lives under, and
`schemaname.Normalize()` accepts any package name. Built-in schemas all sit
under `pudl/` (e.g. `pudl/core.#Item`, `pudl/aws/ec2.#Instance`), and the
legacy short form `core.#Item` auto-normalizes to `pudl/core.#Item`, but there
was no documented guidance for where users should put their own bespoke
schemas.

## Change

`docs/schema-authoring.md` — added a **Namespace Convention** subsection under
"Schema File Location" establishing the recommendation:

- Use `user` as the default top-level package (`user/git.#Repository`,
  `user/k8s.#CustomResource`).
- Any valid CUE package name is allowed (org/team/domain/product names).
- Multiple top-level names may be mixed freely; you are not limited to one.
- Avoid `pudl/`, which is reserved for built-ins and is the auto-normalization
  target for legacy short names — placing schemas there risks shadowing
  built-ins (first-found-wins).

Aligned surrounding examples with the new default:

- Updated the directory tree to show a `user/` package (with `git.cue` and
  `k8s.cue`) alongside the `pudl/` built-ins, and annotated `pudl/` as reserved.
- Switched the `pudl schema new --path ...` examples (basic, enum, collection)
  from the `mypackage/` placeholder to `user/`.
- Switched the manual `pudl schema add` example to `user.my-resource`.
- Left the full custom-schema example using `package myapi` deliberately, as a
  demonstration of the "any name of your choice" point.

## Doctor check

Added a `pudl doctor` health check that warns when user-authored schemas are
placed under the reserved `pudl/` namespace.

- `internal/doctor/checks.go` — `CheckPudlNamespaceSchemas() *CheckResult`.
  Walks `<schemaPath>/pudl/` for `.cue` files, derives each file's package path
  (`filepath.Rel` from the schema root), and flags any package dir not present
  in `importer.BootstrapPackages()` (the authoritative built-in set). Returns a
  `warning` listing the offending packages with a fix pointing at the `user/`
  convention; `ok` when only built-ins are present or `pudl/` does not exist.
  New import: `internal/importer` (no cycle — importer does not import doctor).
- `cmd/doctor.go` — registered the check as "Schema Namespace" and added it to
  the command's long description.
- `internal/doctor/checks_test.go` (new) — `TestCheckPudlNamespaceSchemas`
  covers three cases (no `pudl/` dir, built-ins only, user schema present). Uses
  `t.Setenv("HOME", tmp)` so `config.Load()` resolves the schema path under a
  temp dir.

## Verification

`CGO_ENABLED=0 go build ./...` and `CGO_ENABLED=0 go test ./...` both green.
