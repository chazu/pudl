package glojure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"pudl/op"
)

// coreFunc is a convenience wrapper for simple Go functions implementing CustomFunction.
type coreFunc struct {
	fn func(ctx context.Context, args []interface{}) (interface{}, error)
}

func (f *coreFunc) Execute(ctx context.Context, args []interface{}) (interface{}, error) {
	return f.fn(ctx, args)
}

func newCoreFunc(fn func(ctx context.Context, args []interface{}) (interface{}, error)) op.CustomFunction {
	return &coreFunc{fn: fn}
}

// uppercaseFunc converts a string to uppercase.
func uppercaseFunc(_ context.Context, args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("uppercase expects 1 argument, got %d", len(args))
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("uppercase expects string, got %T", args[0])
	}
	return strings.ToUpper(s), nil
}

// lowercaseFunc converts a string to lowercase.
func lowercaseFunc(_ context.Context, args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("lowercase expects 1 argument, got %d", len(args))
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("lowercase expects string, got %T", args[0])
	}
	return strings.ToLower(s), nil
}

// concatFunc joins strings together.
func concatFunc(_ context.Context, args []interface{}) (interface{}, error) {
	if len(args) == 0 {
		return "", nil
	}
	var parts []string
	for i, a := range args {
		s, ok := a.(string)
		if !ok {
			return nil, fmt.Errorf("concat expects string arguments, got %T at position %d", a, i)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ""), nil
}

// formatFunc wraps fmt.Sprintf.
func formatFunc(_ context.Context, args []interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("format expects at least 1 argument")
	}
	fmtStr, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("format expects string as first argument, got %T", args[0])
	}
	return fmt.Sprintf(fmtStr, args[1:]...), nil
}

// nowFunc returns the current time as an ISO 8601 string.
func nowFunc(_ context.Context, args []interface{}) (interface{}, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("now expects 0 arguments, got %d", len(args))
	}
	return time.Now().UTC().Format(time.RFC3339), nil
}

// envFunc reads an environment variable.
func envFunc(_ context.Context, args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("env expects 1 argument, got %d", len(args))
	}
	name, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("env expects string argument, got %T", args[0])
	}
	return os.Getenv(name), nil
}

// registerCoreNamespace registers pudl.core functions into the registry and
// exposes them to the Glojure runtime.
func registerCoreNamespace(registry *Registry) error {
	rt := registry.rt
	coreFuncs := map[string]func(context.Context, []interface{}) (interface{}, error){
		"uppercase": uppercaseFunc,
		"lowercase": lowercaseFunc,
		"concat":    concatFunc,
		"format":    formatFunc,
		"now":       nowFunc,
		"env":       envFunc,
	}

	for name, fn := range coreFuncs {
		fnCopy := fn // capture for closure
		if err := rt.RegisterGoFunc("pudl.core", name, func(args ...interface{}) interface{} {
			result, err := fnCopy(context.Background(), args)
			if err != nil {
				panic(err)
			}
			return result
		}); err != nil {
			return fmt.Errorf("registering pudl.core/%s: %w", name, err)
		}
	}

	return nil
}
