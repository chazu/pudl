package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// counter ensures unique function names across method invocations.
var invocationCounter uint64

// methodFilePath returns the path to a method's .clj file.
// Convention: <methodsDir>/<model-metadata-name>/<method-name>.clj
func (e *Executor) methodFilePath(modelMetadataName, methodName string) string {
	return filepath.Join(e.methodsDir, modelMetadataName, methodName+".clj")
}

// loadAndRun loads a .clj method file, evaluates it, and calls its (run args) function.
//
// To avoid collisions between multiple method executions in the same runtime,
// each invocation rewrites `(defn run ...)` to a unique function name and
// calls that instead.
func (e *Executor) loadAndRun(ctx context.Context, modelMetadataName, methodName string, args map[string]interface{}) (interface{}, error) {
	path := e.methodFilePath(modelMetadataName, methodName)

	code, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("method file not found: %s", path)
		}
		return nil, fmt.Errorf("reading method file %s: %w", path, err)
	}

	// Generate a unique function name for this invocation
	id := atomic.AddUint64(&invocationCounter, 1)
	uniqueName := fmt.Sprintf("pudl__method__%s__%s__%d",
		sanitizeNS(modelMetadataName), sanitizeNS(methodName), id)

	// Rewrite (defn run [...] ...) to (defn <unique> [...] ...)
	src := strings.Replace(string(code), "(defn run ", fmt.Sprintf("(defn %s ", uniqueName), 1)

	_, err = e.runtime.Eval(src)
	if err != nil {
		return nil, fmt.Errorf("evaluating method %s/%s: %w", modelMetadataName, methodName, err)
	}

	// Register the args as a var the function can access, then call it.
	// We use RegisterGoFunc to make the args available, then Eval to call.
	argsVarName := uniqueName + "__args"
	err = e.runtime.RegisterGoFunc("pudl.executor", argsVarName, func(callArgs ...interface{}) interface{} {
		return args
	})
	if err != nil {
		return nil, fmt.Errorf("registering args for %s/%s: %w", modelMetadataName, methodName, err)
	}

	// Call the function by evaluating (<uniqueName> (pudl.executor/<argsVarName>))
	callExpr := fmt.Sprintf("(%s (pudl.executor/%s))", uniqueName, argsVarName)
	result, err := e.runtime.Eval(callExpr)
	if err != nil {
		return nil, fmt.Errorf("calling %s/%s: %w", modelMetadataName, methodName, err)
	}

	return result, nil
}

// sanitizeNS replaces characters invalid in Clojure identifiers.
func sanitizeNS(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
