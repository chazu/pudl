# pithdriver: pudl driver words for pith VM

Date: 2026-05-11

## Summary

Created `internal/pithdriver/` package that bridges pith VM's untyped stack
with pudl's typed Go APIs. Also added field ref resolution to pith VM itself.

## pith VM changes (~/dev/go/pith/)

- `VM.SetContext(name, data)` — registers named context maps
- `resolveFieldRef` — fallback in Run dispatch resolves `"input.host"` against context maps
- 7 new tests for field ref resolution

## pudl pithdriver package

### Public API

```go
pithdriver.Register(vm *pith.VM, db *database.CatalogDB, mgr *schema.Manager)
```

Registers catalog/*, fact/*, and schema/* driver vocabularies. Pass nil for unused components.

### Driver words

- `catalog/query` ( filters -- [entries] )
- `catalog/get` ( id -- entry ) — tries proquint then raw ID
- `catalog/count` ( filters -- n )
- `fact/query` ( pattern -- [facts] )
- `fact/assert` ( subj pred obj -- )
- `fact/retract` ( id -- )
- `schema/list` ( -- schemas )

### Type conversion

Generic JSON round-trip helpers:
- `mapToStruct[T](map[string]any) (T, error)`
- `structToMap(any) (map[string]any, error)`
- `structsToMaps[T]([]T) ([]any, error)`

### Files

- `internal/pithdriver/register.go` — entry point
- `internal/pithdriver/convert.go` — JSON round-trip helpers
- `internal/pithdriver/catalog.go` — catalog words
- `internal/pithdriver/facts.go` — fact words
- `internal/pithdriver/schema.go` — schema words

### Dependencies

- `github.com/chazu/pith` added to go.mod with `replace => ../pith` for local dev
