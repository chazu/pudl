package glojure

import (
	"sync"
	"time"

	"pudl/op"
)

const defaultTimeout = 30 * time.Second

// FuncEntry describes a registered function (Go or Glojure).
type FuncEntry struct {
	Name      string
	Impl      op.CustomFunction
	Source    string        // "go" or "glojure"
	Cacheable bool
	Timeout   time.Duration
}

// FuncOption configures a function registration.
type FuncOption func(*FuncEntry)

// WithTimeout sets the execution timeout for a function.
func WithTimeout(d time.Duration) FuncOption {
	return func(e *FuncEntry) { e.Timeout = d }
}

// WithCacheable sets whether function results can be cached.
func WithCacheable(v bool) FuncOption {
	return func(e *FuncEntry) { e.Cacheable = v }
}

// Registry is a unified function registry supporting both Go and Glojure functions.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]FuncEntry
	rt      *Runtime
}

// NewRegistry creates a registry backed by the given Glojure runtime.
func NewRegistry(rt *Runtime) *Registry {
	return &Registry{
		entries: make(map[string]FuncEntry),
		rt:      rt,
	}
}

// RegisterGo registers a Go-implemented CustomFunction.
func (r *Registry) RegisterGo(name string, fn op.CustomFunction, opts ...FuncOption) {
	entry := FuncEntry{
		Name:    name,
		Impl:    fn,
		Source:  "go",
		Timeout: defaultTimeout,
	}
	for _, o := range opts {
		o(&entry)
	}

	r.mu.Lock()
	r.entries[name] = entry
	r.mu.Unlock()
}

// RegisterGlojure registers a Glojure function as a CustomFunction via the adapter.
func (r *Registry) RegisterGlojure(name, ns, fnName string, opts ...FuncOption) {
	adapter := &op.GlojureFunc{
		Runtime:  r.rt,
		NS:       ns,
		FuncName: fnName,
	}
	entry := FuncEntry{
		Name:    name,
		Impl:    adapter,
		Source:  "glojure",
		Timeout: defaultTimeout,
	}
	for _, o := range opts {
		o(&entry)
	}

	r.mu.Lock()
	r.entries[name] = entry
	r.mu.Unlock()
}

// Get retrieves a function entry by name.
func (r *Registry) Get(name string) (FuncEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return e, ok
}

// List returns all registered function entries.
func (r *Registry) List() []FuncEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]FuncEntry, 0, len(r.entries))
	for _, e := range r.entries {
		result = append(result, e)
	}
	return result
}
