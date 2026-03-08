package op

import (
	"context"
	"fmt"
)

// GlojureCaller is the interface needed to call Glojure functions.
// Defined here to avoid circular imports (the glojure package depends on op).
type GlojureCaller interface {
	CallFunc(ns, name string, args ...interface{}) (interface{}, error)
}

// GlojureFunc adapts a Glojure function to the CustomFunction interface.
type GlojureFunc struct {
	Runtime  GlojureCaller
	NS       string
	FuncName string
}

// Execute calls the underlying Glojure function.
func (g *GlojureFunc) Execute(ctx context.Context, args []interface{}) (interface{}, error) {
	if g.Runtime == nil {
		return nil, fmt.Errorf("glojure runtime not initialized")
	}

	// Check context before calling
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	result, err := g.Runtime.CallFunc(g.NS, g.FuncName, args...)
	if err != nil {
		return nil, fmt.Errorf("glojure function %s/%s failed: %w", g.NS, g.FuncName, err)
	}
	return result, nil
}
