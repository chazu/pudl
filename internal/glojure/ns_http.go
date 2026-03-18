package glojure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// httpGetFunc performs an HTTP GET and returns the response body as a string.
func httpGetFunc(ctx context.Context, args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("http/get expects 1 argument (url), got %d", len(args))
	}
	url, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("http/get expects string url, got %T", args[0])
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("http/get: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http/get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http/get read body: %w", err)
	}
	return string(body), nil
}

// httpGetJSONFunc performs an HTTP GET and parses the JSON response.
func httpGetJSONFunc(ctx context.Context, args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("http/get-json expects 1 argument (url), got %d", len(args))
	}
	url, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("http/get-json expects string url, got %T", args[0])
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("http/get-json: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http/get-json: %w", err)
	}
	defer resp.Body.Close()

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("http/get-json decode: %w", err)
	}
	return result, nil
}

// httpPostFunc performs an HTTP POST with a string body and returns the response body.
func httpPostFunc(ctx context.Context, args []interface{}) (interface{}, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("http/post expects 2-3 arguments (url, body, [content-type]), got %d", len(args))
	}
	url, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("http/post expects string url, got %T", args[0])
	}
	body, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("http/post expects string body, got %T", args[1])
	}

	contentType := "application/json"
	if len(args) == 3 {
		ct, ok := args[2].(string)
		if !ok {
			return nil, fmt.Errorf("http/post expects string content-type, got %T", args[2])
		}
		contentType = ct
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http/post: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http/post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http/post read body: %w", err)
	}
	return string(respBody), nil
}

// httpStatusFunc performs an HTTP HEAD and returns the status code.
func httpStatusFunc(ctx context.Context, args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("http/status expects 1 argument (url), got %d", len(args))
	}
	url, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("http/status expects string url, got %T", args[0])
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, fmt.Errorf("http/status: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http/status: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// registerHTTPNamespace registers pudl.http functions into the registry and
// exposes them to the Glojure runtime.
func registerHTTPNamespace(registry *Registry) error {
	rt := registry.rt
	httpFuncs := map[string]func(context.Context, []interface{}) (interface{}, error){
		"get":      httpGetFunc,
		"get-json": httpGetJSONFunc,
		"post":     httpPostFunc,
		"status":   httpStatusFunc,
	}

	for name, fn := range httpFuncs {
		fnCopy := fn
		if err := rt.RegisterGoFunc("pudl.http", name, func(args ...interface{}) interface{} {
			result, err := fnCopy(context.Background(), args)
			if err != nil {
				panic(err)
			}
			return result
		}); err != nil {
			return fmt.Errorf("registering pudl.http/%s: %w", name, err)
		}
	}

	return nil
}
