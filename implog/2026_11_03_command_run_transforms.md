# Per-run transforms in CUE (pudl-q4v.2)

**Date:** 2026-11-03

## Work done

A collector run can reshape each record it prints, in CUE, before `set`
applies and before the record is routed and redacted. The GCP Cloud Asset
Inventory collectors need this to unwrap `versionedResources` and to drop
types they do not keep; any tool whose output differs from a schema's shape
can use it.

```cue
runs: [{
	argv: ["gcloud", "asset", "search-all-resources", ...]
	set: project: "{{item}}"
	transform: {
		in: _
		out: [if in.assetType != "compute.googleapis.com/Instance" {null},
			in.versionedResources[0].resource][0]
	}
}]
```

### Measured first

On 5,000 synthetic 3 KB CAI instance records: about 0.9 s for an unwrap and
1.1 s for a reshape. The first implementation compiled each record as CUE
source (`CompileBytes`) and leaked about 54 KB per record in the CUE runtime
(1 GB after 20,000 records). `Context.Encode` did not leak but turned
`json.Number` into strings. The shipped version builds each record from a
JSON syntax tree (`encoding/json.Extract` + `BuildExpr`): heap stays at about
5 MB over 20,000 records, and integers beyond int64 and numbers beyond
float64 survive exactly.

## Public API

**CUE** (`#CommandRun`): `transform?: {in: _, out?: _}`. The struct is
closed; hidden helper fields are allowed. The loader requires `out`.

**Go** (`internal/systemmodel`):

- `type Transform` and `CommandRun.Transform *Transform` (`json:"-"`: read
  from the CUE value, not recorded with the model instance)
- `NewTransform(v cue.Value) (*Transform, error)`: requires `out`
- `(*Transform).Apply(record map[string]any) (out map[string]any, keep bool, err error)`:
  `keep` is false for `out: null`; an `out` that fails, is not concrete, or
  is not an object is an error
- `ModelTemplate.Model()` validates concreteness everywhere except
  `populate.runs[*].transform`, where `in` is a placeholder
- `CommandRun.ForItem` keeps the transform for enumerated runs

**Populate** (`internal/populate`): a record the transform rejects fails its
run with `stdout record N: transform: ...`; other runs are kept.

## Files

- `internal/systemmodel/transform.go` (new), `systemmodel.go`,
  `enumeration.go`, `template.go`, `schema.cue`
- `internal/populate/command.go`
- Docs: `docs/observation-scope.md` (new section), `skills/pudl-core/SKILL.md`
  and its embedded copy; design doc §3 updated with the measurement
- The repository workspace's built-in `systemmodel.cue` refreshed

## Tests

- `internal/systemmodel/transform_test.go`: unwrap with exact numbers, null
  drops, four error cases, loading a model with a transform (kept through
  `ForItem`, not in the recorded JSON), `out` required, closed struct.
- `internal/systemmodel/template_test.go`: incomplete argv, set, timeout and
  name still fail validation; a transform passes.
- `internal/populate/transform_test.go`: transform before set, a transform
  error is a failed input, transforms survive `each`.
- `cmd/command_transform_e2e_test.go`: `pudl model validate` and
  `pudl check` on a CAI-shaped model; the route is dropped and the unwrapped
  firewall fails its check.
- `transform_bench_test.go` (benchmarks) and `transform_mem_test.go`
  (retained heap, run with `PUDL_TRANSFORM_MEM=1`).
