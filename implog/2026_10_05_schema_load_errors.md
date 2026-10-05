# Schema load errors are reported, not silently swallowed

Date: October 5, 2026.

One broken schema package used to fail its whole schema path, and both the
inferrer and the chain validator skipped a failed path with a bare `continue`.
The visible effect was that every import fell back to the catch-all schema,
which looks exactly like "my data did not match anything". Four related
defects are fixed.

## Per-package loading

`CUEModuleLoader` now loads each package independently. A package that fails
to load, build, or register is recorded as a `SchemaLoadError` and skipped;
the remaining packages load. Missing-dependency failures still trigger one
`cue mod tidy` and retry; if tidy fails, the affected packages are reported
instead of failing the path.

A load with any error is still never cached, as before: the user may fix the
problem between attempts, and a fetched dependency changes nothing under the
schema directory, so the fingerprint alone would not notice.

`LoadAllModules` keeps its strict contract (any broken package is an error) for
callers that must not see a partial tree, such as resolving a named model in
`cmd/run_resolve.go` and `cmd/model_list.go`. Tolerant callers use
`LoadModules`.

## Modules keyed by directory, not CUE package name

Modules were stored under `inst.PkgName`, so `first/k8s` and `second/k8s`
(both package `k8s`) overwrote each other and one package's schemas vanished.
They are now keyed by the schema-root-relative directory, which was already the
namespace used for canonical schema names.

## One merged schema set, with integrity checked across paths

The inferrer and chain validator each carried a copy of the same
first-found-wins merge loop. Both now use `validator.LoadSchemaSet`, which also
reports problems instead of discarding paths:

- Schemas that fail `Validate()` are reported per schema; previously the chain
  validator dropped the whole path.
- Missing `base_schema` references are checked across the merged set. The old
  per-path check made the chain validator drop a repository path whenever one
  of its schemas extended a *global* base schema.
- A schema path that does not exist, or contains no CUE packages, is not an
  error: the global schema directory is optional.

## base_schema cycles

`ChainValidator.buildValidationChain` advanced through `base_schema` even when
it stopped appending, so an A → B → A cycle looped forever. It now tracks
visited schemas and `ValidateChain` returns
`base_schema cycle: cyc.#A → cyc.#B → cyc.#A`. The inheritance graph's chain
walk (`GetCascadeChain`, used for identity roots and specificity) stops before
revisiting a schema instead of relying on a 100-step cap.
`validator.FindBaseSchemaCycles` reports every cycle once, starting at its
smallest member.

## Surfacing

- `pudl doctor` has a new **Schema Loading** check (error status) listing each
  failed package, integrity problem, and cycle.
- `pudl import` prints one stderr line when problems exist:
  `warning: 1 schema load error; affected data falls back to the catch-all schema (run 'pudl doctor' for details)`.

Checked against this repository's `.pudl/schema` (66 schemas) and the global
`~/.pudl/schema` (54 schemas): neither reports any problem, so existing
workspaces see no new warnings.

## Not changed: the two validity checks

Inference accepts a match with `Validate(cue.Concrete(true))` (non-list
schemas), while the chain validator accepts `Validate()`. They are not unified
because they answer different questions: inference must not claim a schema
whose required fields the data does not supply concretely, while the chain
validator checks a schema the user named. Changing either moves schema
assignments for existing data, so it is not a safe mechanical change; it needs
its own decision.

## Public API

- `validator.SchemaLoadError{Path, Package, Err}` (`Error`, `Unwrap`).
- `validator.ErrNoInstances`.
- `validator.SchemaSet` and `validator.LoadSchemaSet(paths []string) *SchemaSet`.
- `validator.FindBaseSchemaCycles(map[string]SchemaMetadata) [][]string`,
  `validator.FormatCycle([]string) string`.
- `(*CUEModuleLoader).LoadModules() (map[string]*LoadedModule, []SchemaLoadError, error)`,
  `(*CUEModuleLoader).LoadErrors() []SchemaLoadError`,
  `(*CUEModuleLoader).CheckModuleIntegrity(modules) []SchemaLoadError`.
- `(*ChainValidator).LoadErrors()`, `(*ChainValidator).BaseSchemaCycles()`.
- `(*SchemaInferrer).LoadErrors()`, `(*SchemaInferrer).BaseSchemaCycles()`.
- `inference.WarnLoadErrors(w io.Writer, schemaPaths ...string) bool`.
- `doctor.CheckSchemaLoading()`, `doctor.CheckSchemaLoadingAt(pudlDir string)`.

## Files

- `internal/validator/load_errors.go`, `internal/validator/schema_set.go` (new).
- `internal/validator/cue_loader.go`, `module_cache.go`, `chain_validator.go`.
- `internal/inference/inference.go`, `graph.go`, `load_warnings.go` (new).
- `internal/doctor/schema_load_check.go` (new); registered in `cmd/doctor.go`.
- `cmd/import.go`: one `inference.WarnLoadErrors` call (plus its import).
- Tests: `internal/validator/schema_load_errors_test.go`,
  `internal/inference/load_warnings_test.go`,
  `internal/doctor/schema_load_check_test.go`.
