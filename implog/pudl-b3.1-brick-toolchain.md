# B3.1 — BRICK-Aware Export Actions

## Summary

Added BRICK-aware toolchain mapping to the mu bridge export pipeline. When a definition uses a `brick.#Target` (or `brick.#Kit` / `brick.#Interface`) schema, the `toolchain` field from the declared state is used directly in mu.json output, bypassing the prefix heuristic. Additionally, the `config` sub-field from BRICK targets is used as the mu target config instead of the full declared keys.

## Files Modified

- `internal/mubridge/export.go` — Added `BrickToolchain` and `BrickConfig` fields to `DriftInput`; updated `ExportMuConfig` to check `BrickToolchain` before prefix resolution and use `BrickConfig` when available.
- `internal/mubridge/export_test.go` — Added 4 tests: `TestExportMuConfig_BrickToolchain`, `TestExportMuConfig_BrickToolchainPrecedence`, `TestExportMuConfig_NoBrickFallback`, `TestExportMuConfig_BrickConfig`.
- `cmd/export_actions.go` — Added `buildDriftInput()` helper and `isBrickTarget()` function; updated `runExportOne` and `runExportAll` to use the helper.

## Public API

```go
// DriftInput (updated)
type DriftInput struct {
    Result         *drift.DriftResult
    SchemaRef      string
    Sources        []string
    BrickToolchain string         // explicit toolchain from BRICK metadata (takes precedence)
    BrickConfig    map[string]any // config sub-field from BRICK target (replaces DeclaredKeys)
}
```

## Behavior

- `BrickToolchain` takes absolute precedence over prefix-based toolchain resolution.
- `BrickConfig`, when non-nil, replaces `DeclaredKeys` as the source of mu target config fields.
- Non-BRICK definitions are unaffected; the prefix heuristic continues to work as before.
