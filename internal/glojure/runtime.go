package glojure

import (
	"fmt"
	"sync"

	// Blank import triggers glj.init() which bootstraps the Glojure runtime
	// (loads clojure.core, sets up GlobalEnv, pushes thread bindings).
	_ "github.com/glojurelang/glojure/pkg/glj"
	"github.com/glojurelang/glojure/pkg/lang"
	"github.com/glojurelang/glojure/pkg/runtime"
)

// Runtime manages the Glojure runtime lifecycle.
type Runtime struct {
	initOnce sync.Once
	initErr  error
	mu       sync.Mutex
}

// New creates a new Glojure runtime. Call Init() before use.
func New() *Runtime {
	return &Runtime{}
}

// Init initializes the Glojure runtime. Safe to call multiple times.
func (r *Runtime) Init() error {
	r.initOnce.Do(func() {
		r.initErr = r.doInit()
	})
	return r.initErr
}

func (r *Runtime) doInit() (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("glojure init panic: %v", rec)
		}
	}()

	// Initialize the default environment by evaluating a no-op.
	// This triggers Glojure's internal bootstrap (loading clojure.core, etc.).
	runtime.ReadEval("nil")
	return nil
}

// Eval evaluates a string of Glojure code and returns the result.
func (r *Runtime) Eval(code string) (result interface{}, err error) {
	if err := r.Init(); err != nil {
		return nil, fmt.Errorf("runtime not initialized: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("glojure eval error: %v", rec)
			result = nil
		}
	}()

	result = runtime.ReadEval(code)
	return result, nil
}

// RegisterGoFunc registers a Go function in a Glojure namespace, making it
// callable from Glojure code as (namespace/name ...).
func (r *Runtime) RegisterGoFunc(ns, name string, fn func(...interface{}) interface{}) error {
	if err := r.Init(); err != nil {
		return fmt.Errorf("runtime not initialized: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	namespace := lang.FindOrCreateNamespace(lang.NewSymbol(ns))
	wrapped := lang.NewFnFunc(func(args ...any) any {
		// Convert []any to []interface{} (they're the same underlying type)
		return fn(args...)
	})
	namespace.InternWithValue(lang.NewSymbol(name), wrapped, true)
	return nil
}

// CallFunc calls a function defined in a Glojure namespace from Go.
func (r *Runtime) CallFunc(ns, name string, args ...interface{}) (result interface{}, err error) {
	if err := r.Init(); err != nil {
		return nil, fmt.Errorf("runtime not initialized: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("glojure call %s/%s error: %v", ns, name, rec)
			result = nil
		}
	}()

	namespace := lang.FindNamespace(lang.NewSymbol(ns))
	if namespace == nil {
		return nil, fmt.Errorf("namespace %q not found", ns)
	}

	sym := lang.NewSymbol(name)
	v := namespace.FindInternedVar(sym)
	if v == nil {
		return nil, fmt.Errorf("var %s/%s not found", ns, name)
	}

	fn, ok := v.Get().(lang.IFn)
	if !ok {
		return nil, fmt.Errorf("%s/%s is not a function", ns, name)
	}

	// Convert args to []any for Invoke
	anyArgs := make([]any, len(args))
	for i, a := range args {
		anyArgs[i] = a
	}

	result = fn.Invoke(anyArgs...)
	return result, nil
}
