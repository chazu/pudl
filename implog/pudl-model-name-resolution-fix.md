# Fix: Model Name Resolution — CUE Package Name vs Filesystem Path

## Problem
All methods and workflows failed with "model not found" for every model, including built-in ones. The runtime could not map CUE type references back to discovered models.

## Root Cause
`internal/model/discovery.go:parseModelsFromFile` constructed model names from the filesystem relative path (e.g. `pudl/model/examples.#SimpleModel`), but CUE definitions reference models by their CUE package name (e.g. `examples.#SimpleModel`). The executor's `GetModel()` lookup always failed because of this prefix mismatch.

## Changes
- `internal/model/discovery.go` — Added `extractCUEPackageName()` to parse the `package` declaration from CUE file content. Model names now use the declared CUE package name instead of the filesystem directory path.
- `internal/model/model_test.go` — Updated test expectations from `pudl/model/examples.#X` to `examples.#X`.

## Public API
No API changes. `model.Discoverer.GetModel(name)` and `ListModels()` now return models with CUE-package-based names, which is what all callers (executor, workflow runner) already expected.
