package uiserver

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/graphexport"
)

func testHandler(graph GraphFunc) http.Handler {
	assets := fstest.MapFS{
		"index.html":      {Data: []byte(`<div id="root"></div>`)},
		"assets/app-1.js": {Data: []byte(`console.log(1)`)},
	}
	if graph == nil {
		graph = func(context.Context) (*graphexport.Graph, error) {
			return &graphexport.Graph{Meta: graphexport.Meta{Origin: "pudl", Label: "x"}, Resources: []graphexport.Resource{{ID: "r1"}}}, nil
		}
	}
	return Handler(Options{Assets: assets, Graph: graph, Workspace: "ws"})
}

func get(t *testing.T, h http.Handler, host, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGraphIsLiveAndUncached(t *testing.T) {
	rec := get(t, testHandler(nil), "127.0.0.1:7468", "/api/graph")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var g graphexport.Graph
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &g))
	require.Equal(t, "live", g.Meta.Origin)
	require.Len(t, g.Resources, 1)
}

func TestGraphErrorsAreJSON(t *testing.T) {
	h := testHandler(func(context.Context) (*graphexport.Graph, error) { return nil, errors.New("catalog locked") })
	rec := get(t, h, "localhost:7468", "/api/graph")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "catalog locked")
}

func TestHealthIdentifiesPudl(t *testing.T) {
	rec := get(t, testHandler(nil), "[::1]:7468", "/api/health")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"pudl":true,"workspace":"ws","version":""}`, rec.Body.String())
}

func TestServesBundleWithSPAFallback(t *testing.T) {
	h := testHandler(nil)
	rec := get(t, h, "localhost", "/assets/app-1.js")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Cache-Control"), "immutable")
	for _, path := range []string{"/", "/some/deep/link"} {
		rec = get(t, h, "localhost", path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		require.Contains(t, rec.Body.String(), `id="root"`, path)
	}
	require.Equal(t, http.StatusNotFound, get(t, h, "localhost", "/api/nope").Code)
}

func TestRejectsForeignHosts(t *testing.T) {
	h := testHandler(nil)
	for _, host := range []string{"evil.example:7468", "attacker.test", "10.0.0.5:7468"} {
		require.Equal(t, http.StatusForbidden, get(t, h, host, "/api/graph").Code, host)
	}
	allowed := Handler(Options{Assets: fstest.MapFS{"index.html": {}}, Graph: nil, AllowedHosts: []string{"box.lan:7468"}})
	require.Equal(t, http.StatusOK, get(t, allowed, "box.lan:7468", "/").Code)
	require.Equal(t, http.StatusForbidden, get(t, allowed, "localhost:7468", "/").Code, "an explicit allow list replaces the loopback default")
}

func TestServeStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, testHandler(nil)) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/api/health")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cancel()
	require.NoError(t, <-done)
}

func TestGraphIsCachedPerStampAndVersionIsExposed(t *testing.T) {
	builds := 0
	stamp := "v1"
	h := Handler(Options{
		Assets: fstest.MapFS{"index.html": {}},
		Graph: func(context.Context) (*graphexport.Graph, error) {
			builds++
			return &graphexport.Graph{}, nil
		},
		Stamp: func(context.Context) (string, error) { return stamp, nil },
	})
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, get(t, h, "localhost", "/api/graph").Code)
	}
	require.Equal(t, 1, builds, "an unchanged stamp reuses the graph")
	var g graphexport.Graph
	require.NoError(t, json.Unmarshal(get(t, h, "localhost", "/api/graph").Body.Bytes(), &g))
	require.Equal(t, "v1", g.Meta.Version)
	require.JSONEq(t, `{"version":"v1"}`, get(t, h, "localhost", "/api/version").Body.String())

	stamp = "v2"
	get(t, h, "localhost", "/api/graph")
	require.Equal(t, 2, builds, "a new stamp rebuilds")
}

func TestVersionWithoutStampIs404(t *testing.T) {
	require.Equal(t, http.StatusNotFound, get(t, testHandler(nil), "localhost", "/api/version").Code)
}

func TestFileTreeStamp(t *testing.T) {
	dir := t.TempDir()
	a := FileTreeStamp(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "r.cue"), []byte("x: 1"), 0o644))
	b := FileTreeStamp(dir)
	require.NotEqual(t, a, b)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644))
	require.Equal(t, b, FileTreeStamp(dir), "only .cue files count")
	require.Equal(t, FileTreeStamp(dir), FileTreeStamp(dir, filepath.Join(dir, "missing")))
}
