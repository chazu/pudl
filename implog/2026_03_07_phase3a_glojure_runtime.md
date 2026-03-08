# Phase 3a: Glojure Runtime + CUE Function Unification

**Date:** 2026-03-07

## Summary

Embedded the Glojure runtime into PUDL and unified it with the existing CUE custom function system. CUE functions can now be implemented in either Go or Glojure, and can perform I/O (HTTP requests) during evaluation.

## Changes

### New Files

| File | Purpose |
|------|---------|
| `internal/glojure/runtime.go` | Glojure runtime lifecycle — init, eval, Go func registration, cross-language calls |
| `internal/glojure/registry.go` | Unified function registry supporting both Go and Glojure function implementations |
| `internal/glojure/builtins.go` | Registers builtin namespaces and maps CUE `op.#Name` identifiers to functions |
| `internal/glojure/ns_core.go` | `pudl.core` namespace — uppercase, lowercase, concat, format, now, env |
| `internal/glojure/ns_http.go` | `pudl.http` namespace — get, get-json, post, status |
| `internal/glojure/glojure_test.go` | 15 tests covering runtime, registry, namespaces, caching, timeout, error handling |
| `op/adapter.go` | `GlojureFunc` adapter — bridges Glojure functions to `CustomFunction` interface |

### Modified Files

| File | Change |
|------|--------|
| `go.mod` | Added `github.com/glojurelang/glojure` v0.6.4 |
| `op/functions.go` | `CustomFunction.Execute` now takes `context.Context`; removed concrete implementations and `GetFunction()` |
| `internal/cue/processor.go` | Accepts `*glojure.Registry`; added result caching (SHA256-keyed), per-function timeout, improved error context |
| `cmd/process.go` | Initializes Glojure runtime + registry, registers builtins, passes registry to processor |

## Public API

### `internal/glojure` package

```go
// Runtime lifecycle
func New() *Runtime
func (r *Runtime) Init() error
func (r *Runtime) Eval(code string) (interface{}, error)
func (r *Runtime) RegisterGoFunc(ns, name string, fn func(...interface{}) interface{}) error
func (r *Runtime) CallFunc(ns, name string, args ...interface{}) (interface{}, error)

// Unified registry
func NewRegistry(rt *Runtime) *Registry
func (r *Registry) RegisterGo(name string, fn op.CustomFunction, opts ...FuncOption)
func (r *Registry) RegisterGlojure(name, ns, fnName string, opts ...FuncOption)
func (r *Registry) Get(name string) (FuncEntry, bool)
func (r *Registry) List() []FuncEntry

// Options
func WithTimeout(d time.Duration) FuncOption
func WithCacheable(v bool) FuncOption

// Builtin registration
func RegisterBuiltins(registry *Registry) error
```

### `op` package

```go
// Updated interface
type CustomFunction interface {
    Execute(ctx context.Context, args []interface{}) (interface{}, error)
}

// Adapter
type GlojureFunc struct { Runtime GlojureCaller; NS, FuncName string }
func (g *GlojureFunc) Execute(ctx context.Context, args []interface{}) (interface{}, error)
```

### CUE Function Registry

| CUE Name | Namespace | Cacheable |
|----------|-----------|-----------|
| `#Uppercase` | `pudl.core/uppercase` | yes |
| `#Lowercase` | `pudl.core/lowercase` | yes |
| `#Concat` | `pudl.core/concat` | yes |
| `#Format` | `pudl.core/format` | yes |
| `#Now` | `pudl.core/now` | no |
| `#Env` | `pudl.core/env` | no |
| `#HttpGet` | `pudl.http/get` | no |
| `#HttpGetJson` | `pudl.http/get-json` | no |
| `#HttpPost` | `pudl.http/post` | no |
| `#HttpStatus` | `pudl.http/status` | no |

## Test Results

15 tests passing in `internal/glojure/`:
- Runtime init, eval, defn, Go func registration, cross-language calls
- Registry (Go and Glojure functions), builtin listing
- Core namespace (uppercase, lowercase, concat, format, now)
- HTTP namespace (get, get-json, post, status against httptest server)
- Timeout enforcement, caching behavior, error handling (panic recovery)

All existing tests continue to pass (full suite).
