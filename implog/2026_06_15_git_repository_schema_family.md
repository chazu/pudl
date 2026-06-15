# Git repository schema family (bootstrap)

**Date:** 2026-06-15
**Design doc:** `docs/issues/git-repository-decomposed-resources.md` (D2/D3, scoped)
**Builds on:** `implog/2026_06_15_component_schema_boundary.md` (D1)

## Summary

Added a built-in git-repository schema family under the new bootstrap package
`pudl/git`, per the scoped design decision (no per-branch bitemporal history this
swing → branches and remotes are inline components, C1–C4 deferred):

- **`#GitRepository`** — platform-agnostic family root. Identity `["name"]` (the
  fully-qualified path; git assigns no inherent name). `root_commit` is optional
  ⇒ tracked, not identity (D2). Carries inline `remotes` and `branches`.
- **`#GitRemote`, `#GitBranch`** — inline *components* (no `_pudl` block). Not
  tracked resources; D1 keeps them out of the schema registry. Re-importing a
  repository replaces the whole array, so removed entries drop implicitly.
- **`#GitHubRepository`, `#GitLabRepository`** — platform specializations built
  with CUE unification (`#Child: #GitRepository & {...}`). They inherit
  `identity_fields: ["name"]` unchanged (family-identity invariant), tighten
  `name` to the host pattern (`^github\.com/`, `^gitlab\.com/`), narrow
  `resource_type` for origin-keyword matching, and add optional platform fields
  (`owner`/`namespace`, `visibility`).

Registered all three in the catalog (`pudl/catalog/catalog.cue`).

## Key CUE detail

`_pudl` inside a `#`-definition is **closed**, so a child unified with the base
cannot introduce a `base_schema` field unless the base declares it. The base
`#GitRepository._pudl` therefore declares `base_schema?: string`. `resource_type`
is declared as `string | *"git.repository"` (a default) so specializations can
narrow it without a unification conflict.

## Files

- `internal/importer/bootstrap/pudl/git/git.cue` — new package (family + components).
- `internal/importer/bootstrap/pudl/catalog/catalog.cue` — three git catalog entries.

## Public API

None changed. New bootstrap packages are discovered dynamically by
`importer.BootstrapPackages` (walks the embedded FS), so no registration list
edits were needed.

## Tests

- `internal/importer/git_schema_test.go`:
  - `TestGitFamilyRegistration` — the three schemas register; `#GitRemote`/
    `#GitBranch` do not (D1); specializations' parent is `#GitRepository` and
    their `identity_fields` stay `["name"]`.
  - `TestGitFamilyInference` — a github.com blob → `#GitHubRepository`, a
    gitlab.com blob → `#GitLabRepository`, a local-path blob → base
    `#GitRepository`.
- Full suite green (`CGO_ENABLED=0 go test ./...`).

## Notes / out of scope

- The test loads the bootstrap schemas via `CopyBootstrapSchemas` but first drops
  the bootstrap `definitions/` tree: `definitions/http_def.cue` carries a stale
  `import "pudl.schemas/pudl/model/examples"` (the `model` package was extracted
  to mu and no longer exists), which fails the whole module load. This is
  pre-existing rot in the bootstrap `definitions/` specs, unrelated to the git
  family, and is left untouched here.
