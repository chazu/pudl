// Package uiserver serves the embedded web graph explorer and a read-only
// JSON API over the workspace catalog. It only ever reads: there are no
// write endpoints, in keeping with "observe, never change".
//
// Endpoints:
//
//	GET /api/health   {"pudl": true, "workspace": ...}; the UI probes it to pick its data source
//	GET /api/graph    the current resource graph (graphexport.Graph, meta.origin "live")
//	GET /*            the UI bundle; unknown paths fall back to index.html
//
// Requests whose Host header is not the loopback address the server was
// started on are rejected, so a web page cannot read the inventory through
// DNS rebinding.
package uiserver

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/chazu/pudl/internal/graphexport"
)

// GraphFunc builds the current graph. It is called once per request.
type GraphFunc func(ctx context.Context) (*graphexport.Graph, error)

// Options configures the handler.
type Options struct {
	Assets    fs.FS
	Graph     GraphFunc
	Workspace string
	Version   string
	// AllowedHosts are the Host header values accepted (host[:port]). Empty
	// accepts only loopback names.
	AllowedHosts []string
}

// Handler returns the server's HTTP handler.
func Handler(opts Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"pudl": true, "workspace": opts.Workspace, "version": opts.Version})
	})
	mux.HandleFunc("GET /api/graph", func(w http.ResponseWriter, r *http.Request) {
		g, err := opts.Graph(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		g.Meta.Origin = "live"
		writeJSON(w, http.StatusOK, g)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such endpoint"})
	})
	mux.Handle("/", spa(opts.Assets))
	return hostGuard(opts.AllowedHosts, mux)
}

// spa serves files from assets; any path that is not a file gets index.html.
// Hashed bundles under assets/ are immutable.
func spa(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" {
			if st, err := fs.Stat(assets, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "UI bundle missing; rebuild pudl after `make web`", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func hostGuard(allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, allowed) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hostAllowed(host string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(host, a) {
			return true
		}
	}
	if len(allowed) > 0 {
		return false
	}
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	return IsLoopback(name)
}

// IsLoopback reports whether a host name or IP is loopback-only.
func IsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

// Serve serves h on ln until ctx is cancelled, then shuts down gracefully.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{Handler: h}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
