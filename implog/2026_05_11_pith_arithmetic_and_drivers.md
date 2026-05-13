# Pith Arithmetic Builtins, Transform Output Passing, Deferred Driver Words

Date: 2026-05-11

## Pith Arithmetic Builtins

Added `add`, `sub`, `mul`, `div`, `mod` to pith data.go. All operate on numeric types (int, int64, float64) via `toFloat64` coercion. Division and modulo by zero return errors.

Updated CUE schema (`cue.cue`) with `#ArithOp` union type. Also added `group-by` and `flatten` to `#SeqOp` (were missing from schema).

### Public API

```
add ( a b -- a+b )
sub ( a b -- a-b )
mul ( a b -- a*b )
div ( a b -- a/b )    -- errors on zero divisor
mod ( a b -- a%b )    -- errors on zero divisor
```

Tests: 11 new tests in `data_test.go` (basic ops, chaining, mixed types, div/mod by zero). Total pith tests: 127.

## Transform Output Passing (mu)

Modified `internal/dag/executor.go` to capture pith VM stack results after execution:

- Added `pithResults map[string]any` field to `Executor` (target name -> result)
- After `executePithVM`, stores `vm.Result()` under target prefix
- `getOutput` closure merges `pithResults` into returned map under `_result` key
- Downstream actions access transform result via `target/output` then `"'_result" "get"`

Test: `TestPithBodyResultPropagates` in `executor_test.go`. Total mu tests: 428.

## Deferred Driver Words

### pudl: schema/match, schema/infer

New file `internal/pithdriver/schema_infer.go`. Requires `*inference.SchemaInferrer` (optional in `Register`).

```
schema/match ( data -- schema_name )    -- best match or nil
schema/infer ( data -- result_map )     -- full inference result
```

### pudl: drift/diff

New file `internal/pithdriver/drift.go`. Uses `drift.Compare` for field-level diffing.

```
drift/diff ( declared live -- [diffs] )  -- each diff has path, type, declared, live
```

Test: `TestExampleDriftDiff` in `examples_test.go`. Total pudl tests: 915.

### mu: format/json, format/compact

Added to `internal/pithvm/register.go` in `RegisterExecDrivers`.

```
format/json    ( value -- string )    -- pretty-printed JSON
format/compact ( value -- string )    -- minified JSON
```

### Register signature change

`pithdriver.Register` now takes 4th arg `*inference.SchemaInferrer` (nil-safe). Updated `cmd/exec.go` to create inferrer from schema paths.

## Files Changed

- pith: `data.go`, `cue.cue`, `data_test.go`
- mu: `internal/dag/executor.go`, `internal/dag/executor_test.go`, `internal/pithvm/register.go`
- pudl: `internal/pithdriver/register.go`, `internal/pithdriver/schema_infer.go` (new), `internal/pithdriver/drift.go` (new), `internal/pithdriver/examples_test.go`, `cmd/exec.go`
