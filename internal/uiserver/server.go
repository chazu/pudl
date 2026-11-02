// Package uiserver serves the embedded web graph explorer and a read-only
// JSON API over the workspace catalog. It only ever reads: there are no
// write endpoints, in keeping with "observe, never change".
//
// Endpoints:
//
//	GET /api/health   {"pudl": true, "workspace": ...}; the UI probes it to pick its data source
//	GET /api/version  {"version": stamp}; changes whenever the graph would change
//	GET /api/graph    the current resource graph (graphexport.Graph, meta.origin "live");
//	                  ?as_of=<RFC3339> gives the graph as recorded at that time
//	                  ?focus=<ids>&depth=<0-3>&closure=<types> gives a neighbourhood (graphexport.Subgraph)
//	GET /api/summary  resource and relationship counts (?as_of too)
//	GET /api/search   ?q=&limit= lightweight resource matches (?as_of too)
//	GET /api/snapshots  recent observation snapshots, newest first (as-of quick picks)
//	GET /api/events   Server-Sent Events: "version" with the change stamp on
//	                  connect and whenever it changes (checked every second)
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chazu/pudl/internal/graphexport"
)

// GraphFunc builds the graph now (asOf nil) or as recorded at asOf.
type GraphFunc func(ctx context.Context, asOf *time.Time) (*graphexport.Graph, error)

// Snapshot is one observation snapshot, offered as an as-of point.
type Snapshot struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Model     string    `json:"model,omitempty"`
	Scope     string    `json:"scope,omitempty"`
	Complete  bool      `json:"complete"`
}

// SnapshotsFunc lists recent snapshots, newest first.
type SnapshotsFunc func(ctx context.Context) ([]Snapshot, error)

// StampFunc returns the current change stamp: equal stamps mean an equal
// graph. Nil disables caching and /api/version.
type StampFunc func(ctx context.Context) (string, error)

// Options configures the handler.
type Options struct {
	Assets    fs.FS
	Graph     GraphFunc
	Stamp     StampFunc
	Snapshots SnapshotsFunc
	// EventInterval is how often /api/events checks the stamp (default 1s).
	EventInterval time.Duration
	Workspace     string
	Version       string
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
	cache := &graphCache{build: opts.Graph, stamp: opts.Stamp}
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		if opts.Stamp == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "change tracking unavailable"})
			return
		}
		v, err := opts.Stamp(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"version": v})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		if opts.Stamp == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "change tracking unavailable"})
			return
		}
		streamVersions(w, r, opts.Stamp, opts.EventInterval)
	})
	mux.HandleFunc("GET /api/snapshots", func(w http.ResponseWriter, r *http.Request) {
		list := []Snapshot{}
		if opts.Snapshots != nil {
			got, err := opts.Snapshots(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			list = append(list, got...)
		}
		writeJSON(w, http.StatusOK, list)
	})
	// graphFor resolves the graph a request asks about: now (cached per
	// stamp) or ?as_of=<RFC3339> (built on demand).
	graphFor := func(w http.ResponseWriter, r *http.Request) (*graphexport.Graph, bool) {
		var (
			g   *graphexport.Graph
			err error
		)
		if raw := r.URL.Query().Get("as_of"); raw != "" {
			at, perr := time.Parse(time.RFC3339, raw)
			if perr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "as_of must be RFC 3339: " + perr.Error()})
				return nil, false
			}
			if g, err = opts.Graph(r.Context(), &at); err == nil {
				g.Meta.Origin = "live"
			}
		} else {
			g, err = cache.get(r.Context())
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return nil, false
		}
		return g, true
	}
	mux.HandleFunc("GET /api/graph", func(w http.ResponseWriter, r *http.Request) {
		g, ok := graphFor(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		if focus := splitList(q.Get("focus")); len(focus) > 0 {
			depth := 1
			if d, err := strconv.Atoi(q.Get("depth")); err == nil && d >= 0 && d <= 3 {
				depth = d
			}
			g = graphexport.Subgraph(g, focus, depth, splitList(q.Get("closure")))
		}
		writeJSON(w, http.StatusOK, g)
	})
	mux.HandleFunc("GET /api/summary", func(w http.ResponseWriter, r *http.Request) {
		g, ok := graphFor(w, r)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"resources": len(g.Resources), "relationships": len(g.Relationships), "version": g.Meta.Version,
		})
	})
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		g, ok := graphFor(w, r)
		if !ok {
			return
		}
		limit := 50
		if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 500 {
			limit = l
		}
		writeJSON(w, http.StatusOK, graphexport.Search(g, r.URL.Query().Get("q"), limit))
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such endpoint"})
	})
	mux.Handle("/", spa(opts.Assets))
	return hostGuard(opts.AllowedHosts, mux)
}

// graphCache builds the graph at most once per change stamp. Builds are
// serialized: concurrent requests wait for one build instead of racing.
type graphCache struct {
	build GraphFunc
	stamp StampFunc

	mu      sync.Mutex
	version string
	graph   *graphexport.Graph
}

func (c *graphCache) get(ctx context.Context) (*graphexport.Graph, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var version string
	if c.stamp != nil {
		v, err := c.stamp(ctx)
		if err != nil {
			return nil, err
		}
		if c.graph != nil && v == c.version {
			return c.graph, nil
		}
		version = v
	}
	g, err := c.build(ctx, nil)
	if err != nil {
		return nil, err
	}
	g.Meta.Origin = "live"
	g.Meta.Version = version
	if c.stamp != nil {
		c.graph, c.version = g, version
	}
	return g, nil
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
	// Requests derive from ctx, so long-lived event streams end when it is
	// cancelled and Shutdown does not wait on them forever.
	srv := &http.Server{Handler: h, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
