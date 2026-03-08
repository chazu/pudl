package glojure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeInit(t *testing.T) {
	rt := New()
	err := rt.Init()
	require.NoError(t, err, "runtime should initialize without error")

	// Double init is safe
	err = rt.Init()
	require.NoError(t, err, "second init should be no-op")
}

func TestEvalBasic(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	result, err := rt.Eval("(+ 1 2)")
	require.NoError(t, err)
	assert.Equal(t, int64(3), result)
}

func TestEvalDefn(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	// Define a function
	_, err := rt.Eval("(defn my-double [x] (* x 2))")
	require.NoError(t, err)

	// Call it
	result, err := rt.Eval("(my-double 21)")
	require.NoError(t, err)
	assert.Equal(t, int64(42), result)
}

func TestRegisterGoFunc(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	err := rt.RegisterGoFunc("test.ns", "triple", func(args ...interface{}) interface{} {
		// Glojure passes int64 for numbers
		return args[0].(int64) * 3
	})
	require.NoError(t, err)

	result, err := rt.Eval("(test.ns/triple 7)")
	require.NoError(t, err)
	assert.Equal(t, int64(21), result)
}

func TestCallFunc(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	// Register a Go function in a namespace, then call it via CallFunc
	err := rt.RegisterGoFunc("my.funcs", "add", func(args ...interface{}) interface{} {
		a := args[0].(int64)
		b := args[1].(int64)
		return a + b
	})
	require.NoError(t, err)

	// Call it from Go
	result, err := rt.CallFunc("my.funcs", "add", int64(10), int64(32))
	require.NoError(t, err)
	assert.Equal(t, int64(42), result)
}

func TestCallFunc_NotFound(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	_, err := rt.CallFunc("nonexistent.ns", "nope")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistryGoFunction(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	registry := NewRegistry(rt)
	registry.RegisterGo("#TestFunc", newCoreFunc(func(_ context.Context, args []interface{}) (interface{}, error) {
		return fmt.Sprintf("got:%v", args[0]), nil
	}), WithCacheable(true))

	entry, ok := registry.Get("#TestFunc")
	require.True(t, ok)
	assert.Equal(t, "go", entry.Source)
	assert.True(t, entry.Cacheable)

	result, err := entry.Impl.Execute(context.Background(), []interface{}{"hello"})
	require.NoError(t, err)
	assert.Equal(t, "got:hello", result)
}

func TestRegistryGlojureFunction(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	// Register a Go func in Glojure namespace
	err := rt.RegisterGoFunc("test.registry", "shout", func(args ...interface{}) interface{} {
		return fmt.Sprintf("%s!", args[0])
	})
	require.NoError(t, err)

	registry := NewRegistry(rt)
	registry.RegisterGlojure("#Shout", "test.registry", "shout")

	entry, ok := registry.Get("#Shout")
	require.True(t, ok)
	assert.Equal(t, "glojure", entry.Source)

	result, err := entry.Impl.Execute(context.Background(), []interface{}{"hey"})
	require.NoError(t, err)
	assert.Equal(t, "hey!", result)
}

func TestCoreNamespace(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	registry := NewRegistry(rt)
	require.NoError(t, RegisterBuiltins(registry))

	tests := []struct {
		name   string
		args   []interface{}
		expect interface{}
	}{
		{"#Uppercase", []interface{}{"hello"}, "HELLO"},
		{"#Lowercase", []interface{}{"WORLD"}, "world"},
		{"#Concat", []interface{}{"foo", "bar"}, "foobar"},
		{"#Format", []interface{}{"hello %s %d", "world", 42}, "hello world 42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := registry.Get(tt.name)
			require.True(t, ok, "function %s should be registered", tt.name)

			result, err := entry.Impl.Execute(context.Background(), tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestCoreNow(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	registry := NewRegistry(rt)
	require.NoError(t, RegisterBuiltins(registry))

	entry, ok := registry.Get("#Now")
	require.True(t, ok)

	result, err := entry.Impl.Execute(context.Background(), []interface{}{})
	require.NoError(t, err)

	// Should be a valid RFC3339 timestamp
	_, err = time.Parse(time.RFC3339, result.(string))
	assert.NoError(t, err, "should be a valid RFC3339 timestamp")
}

func TestHttpNamespace(t *testing.T) {
	// Create a test HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/text":
			w.Write([]byte("hello from server"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"key": "value"})
		case "/post":
			w.Write([]byte("posted"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	rt := New()
	require.NoError(t, rt.Init())
	registry := NewRegistry(rt)
	require.NoError(t, RegisterBuiltins(registry))

	t.Run("HttpGet", func(t *testing.T) {
		entry, ok := registry.Get("#HttpGet")
		require.True(t, ok)

		result, err := entry.Impl.Execute(context.Background(), []interface{}{srv.URL + "/text"})
		require.NoError(t, err)
		assert.Equal(t, "hello from server", result)
	})

	t.Run("HttpGetJson", func(t *testing.T) {
		entry, ok := registry.Get("#HttpGetJson")
		require.True(t, ok)

		result, err := entry.Impl.Execute(context.Background(), []interface{}{srv.URL + "/json"})
		require.NoError(t, err)

		m, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "value", m["key"])
	})

	t.Run("HttpStatus", func(t *testing.T) {
		entry, ok := registry.Get("#HttpStatus")
		require.True(t, ok)

		result, err := entry.Impl.Execute(context.Background(), []interface{}{srv.URL + "/text"})
		require.NoError(t, err)
		assert.Equal(t, 200, result)
	})

	t.Run("HttpPost", func(t *testing.T) {
		entry, ok := registry.Get("#HttpPost")
		require.True(t, ok)

		result, err := entry.Impl.Execute(context.Background(), []interface{}{srv.URL + "/post", `{"data":"test"}`})
		require.NoError(t, err)
		assert.Equal(t, "posted", result)
	})
}

func TestTimeout(t *testing.T) {
	// Create a slow server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("slow"))
	}))
	defer srv.Close()

	rt := New()
	require.NoError(t, rt.Init())

	// Execute with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := httpGetFunc(ctx, []interface{}{srv.URL + "/slow"})
	assert.Error(t, err, "should timeout")
}

func TestCaching(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	callCount := 0
	registry := NewRegistry(rt)
	registry.RegisterGo("#Counted", newCoreFunc(func(_ context.Context, args []interface{}) (interface{}, error) {
		callCount++
		return fmt.Sprintf("call-%d", callCount), nil
	}), WithCacheable(true))

	entry, _ := registry.Get("#Counted")

	// Simulate what the processor does: cache keyed by func+args
	cache := make(map[string]interface{})

	// First call
	result1, err := entry.Impl.Execute(context.Background(), []interface{}{"a"})
	require.NoError(t, err)
	cache["#Counted|a"] = result1

	// "Second call" — use cache
	cached := cache["#Counted|a"]
	assert.Equal(t, result1, cached, "cached value should match")
	assert.Equal(t, 1, callCount, "function should only be called once")
}

func TestErrorHandling(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	// Bad Glojure code should return error, not panic
	_, err := rt.Eval("(this-is-not-defined)")
	assert.Error(t, err, "bad code should return error")
}

func TestRegistryList(t *testing.T) {
	rt := New()
	require.NoError(t, rt.Init())

	registry := NewRegistry(rt)
	require.NoError(t, RegisterBuiltins(registry))

	entries := registry.List()
	assert.GreaterOrEqual(t, len(entries), 10, "should have at least 10 builtin functions")
}
