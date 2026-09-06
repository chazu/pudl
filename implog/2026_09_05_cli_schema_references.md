# Retrievable CLI schema references

Date: 2026-09-05
Issue: `pudl-sma`

## Problem and behavior

`pudl model show git-inventory` printed `git.repository` beneath a count of
"definitions." That value is a resource-type routing tag, so passing it to
`pudl schema show` failed its schema-name parser.

The human model display now counts desired resources and resolves their tags
using loaded schema metadata, with workspace schemas taking precedence over
global schemas. The tutorial entry appears as:

```text
  Desired:   1 resource(s)
    - pudl/git.#GitRepository (resource type: git.repository)
```

Explicit CUE references are normalized to `package.#Definition`. If a resource
type has several matching schemas, all names are listed in sorted order. If no
schema is registered, the display says so instead of inventing a definition.

Both normal and verbose `schema list` output prints full names. `schema add`
reports the definitions actually present in the file and uses those names in
inspection/import suggestions; the file slug is labeled as a file. `schema new`
prints full names for generated and reused definitions, inspection/edit commands,
and existing-schema errors, preserving nested package paths.

## Public interface

Human CLI output changes only; no new flags or exported Go API. Model JSON and
authored `_schema` routing tags retain their existing values. Observation routing,
drift resource identities, and stored reports are unchanged.

The tutorial now includes `pudl schema show pudl/git.#GitRepository` and explains
schema names, resource types, and how to inspect the desired values as JSON.

## Validation

- The tutorial acceptance test reproduced the old output before the fix, then
  passed after following the printed schema name into `schema show` and checking
  the repository definition and its data fields.
- Command regressions cover custom resource types, canonical/import-style
  references, ambiguous and missing schemas, workspace shadowing, unchanged model
  JSON, both schema listing modes, and file slugs differing from definitions.
- A fresh-workspace CLI check generated `user/git.#CopiedRepository`, retrieved
  its printed name with `schema show`, and verified that trying to generate it
  again reports the full nested schema name.
- `mise exec -- go test ./...`
- `mise exec -- go vet ./...`
- `mise exec -- make test-git-walkthrough`
- `mise exec -- make check-skills`
- `git diff --check`
