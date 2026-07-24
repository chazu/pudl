# `--only` selector ambiguity fix (Defect 2)

Fixed both halves of the `--only` selector defect recorded in
`docs/architecture-improvement-report.md`. This was the only defect in the
register that could mutate infrastructure the operator never named.

## The defect

`desiredSelectorValues` flattened nine keys — `_schema`, `schema`, `definition`,
`name`, `id`, `path`, `kind`, `target`, `metadata.name` — into a single
namespace, so a match carried no record of *why* it matched.

- **2a, dependency lookup:** the scan took the first candidate whose flattened
  set contained the dependency string and broke. With desired
  `[{name: decoy, kind: nginx}, {name: nginx, kind: Deployment}, {name: app,
  depends_on: nginx}]`, `--only app` pulled `decoy` into converge scope because
  its *kind* was `nginx`, and left out the resource actually named `nginx`.
  Deterministic and order-dependent, therefore silent.
- **2b, top-level selector:** the selection loop had no `break`, so a selector
  matched *every* resource whose flattened namespace contained it. `--only nginx`
  against the same set selected both.

## The fix

Selector matching is now key-class aware. `internal/acute/plan.go`:

- `selectorKind{identity, typed bool}` records how a value matched.
- `typeSelectorKeys` (`_schema`, `schema`, `definition`, `kind`) name a type, so
  matching several resources is the documented intent (`--only Deployment`).
  `identitySelectorKeys` (`name`, `id`, `path`, `target`, plus `metadata.name`)
  name exactly one.
- `resolveSelector` returns the matched indexes or an error. A selector matching
  several resources by identity, or matching some by identity and *different*
  ones by type, is now an error instead of a silent pick. A single resource
  matching by both classes stays legal, since either reading names the same one.
- `resolveDependency` requires exactly one match, because a dependency edge
  points at one resource; a dependency naming a type is an error.
- Error messages name each matched resource by its most specific identifier and
  state which key produced the match (`name=decoy via kind`), so the operator can
  see why an unnamed resource was captured.

Behaviour deliberately preserved: type selectors still select sets, unknown
selectors still fail before side effects, dependency closure is still transitive
and still terminates on cycles.

## Tests

`internal/acute/plan_test.go` — cross-class dependency match, cross-class
selector match, duplicate identity selector, type selector matching many
resources, dependency matching a set, unambiguous transitive closure,
self-consistent dual-class match, cyclic termination.

`CGO_ENABLED=0 go test ./...` and `go vet` pass.

## Public API

`internal/acute` is internal; no exported surface changed. `ScopeModelForRun`
keeps its signature and gains failure modes. The user-facing change is that
previously-silent ambiguous `--only` selectors now error — documented in
`docs/cli-reference.md`.

## Tracker

Filed the full 15-entry defect register as beads issues. This one closed as
fixed; the rest remain open, led by `--from-catalog` running unscoped and
promoting `converging` to `clean` with no observation.
