# Epic 3: BRICK-Aware Toolchain Mapping

## Goal

Make BRICK metadata load-bearing by using `#Target.toolchain` directly instead of prefix heuristics when definitions use BRICK schemas.

## Beads

- B3.1 — BRICK-aware export-actions

## Dependencies

None — independent of other epics.

## Acceptance Criteria

A definition like:

```cue
my_app: brick.#Target & {
    name: "my-app"
    kind: "component"
    toolchain: "k8s"
    config: { ... }
}
```

Produces a mu.json target with `"toolchain": "k8s"` taken from the BRICK field, not inferred from schema prefix.
