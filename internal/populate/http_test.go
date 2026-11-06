package populate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/ingestprep"
	"github.com/chazu/pudl/internal/systemmodel"
)

const testToken = "tok-3f9a"

// apiServer serves canned JSON by request URI and records every request.
type apiServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string // "METHOD URI auth=<Authorization>"
}

func newAPIServer(t *testing.T, routes map[string]string) *apiServer {
	s := &apiServer{}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, fmt.Sprintf("%s %s auth=%s", r.Method, r.URL.RequestURI(), r.Header.Get("Authorization")))
		s.mu.Unlock()
		body, ok := routes[r.URL.RequestURI()]
		if !ok {
			http.Error(w, `{"message":"secret-body: no such thing"}`, http.StatusNotFound)
			return
		}
		if strings.HasPrefix(body, "redirect:") {
			http.Redirect(w, r, strings.TrimPrefix(body, "redirect:"), http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *apiServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *apiServer) request() Request {
	return Request{HTTPClient: s.Client(), Getenv: func(name string) string {
		if name == "TEST_TOKEN" {
			return testToken
		}
		return ""
	}}
}

func cueTransform(t *testing.T, src string) *systemmodel.Transform {
	tr, err := systemmodel.NewTransform(cuecontext.New().CompileString(src))
	require.NoError(t, err)
	return tr
}

func cuePredicate(t *testing.T, src string) *systemmodel.Predicate {
	p, err := systemmodel.NewPredicate(cuecontext.New().CompileString(src))
	require.NoError(t, err)
	return p
}

func httpPopulate(s *apiServer, h systemmodel.HTTPObserve) systemmodel.Populate {
	h.Base = s.URL
	if h.Auth == nil {
		h.Auth = &systemmodel.HTTPAuth{Env: "TEST_TOKEN", Header: "Authorization", Scheme: "token"}
	}
	return systemmodel.Populate{HTTP: &h}
}

// The guard passes, the enumeration follows its continuation token and maps
// each element to an input, and each run's records are selected, transformed
// and given set. Every request is a GET carrying the token.
func TestHTTPRunGuardsEnumeratesAndTransforms(t *testing.T) {
	s := newAPIServer(t, map[string]string{
		"/api/user":                        `{"team":"readers"}`,
		"/api/stacks":                      `{"stacks":[{"org":"acme","name":"a"}],"continuationToken":"p2"}`,
		"/api/stacks?continuationToken=p2": `{"stacks":[{"org":"acme","name":"b"},{"org":"acme","name":"a"}]}`,
		"/api/stacks/acme/a/export":        `{"deployment":{"resources":[{"urn":"u1","inputs":{"password":"x"}},{"urn":"u2"}]}}`,
		"/api/stacks/acme/b/export":        `{"deployment":{"resources":null}}`,
	})
	p := httpPopulate(s, systemmodel.HTTPObserve{
		Guard: &systemmodel.HTTPGuard{Requests: map[string]string{"": "/api/user"}, Check: cuePredicate(t, `{in: _, ok: in.team == "readers"}`)},
		Each: &systemmodel.HTTPEnumeration{Get: "/api/stacks", List: "stacks",
			Page: &systemmodel.HTTPPage{Next: "continuationToken", Param: "continuationToken"},
			Item: cueTransform(t, `{in: _, out: "\(in.org)/\(in.name)"}`)},
		Runs: []systemmodel.HTTPRun{{Get: "/api/stacks/{{item}}/export", Records: "deployment.resources",
			Set: map[string]any{"stack": "{{item}}"}, Transform: cueTransform(t, `{in: _, out: {urn: in.urn}}`)}},
	})
	res, err := Run(context.Background(), p, s.request())
	require.NoError(t, err)
	assert.Equal(t, []string{"acme/a", "acme/b"}, res.Inputs, "inputs are deduplicated across pages")
	assert.Equal(t, 2, res.Runs)
	assert.Empty(t, res.Failed)
	encoded, err := json.Marshal(res.Records)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"urn":"u1","stack":"acme/a"},{"urn":"u2","stack":"acme/a"}]`, string(encoded))
	for _, r := range s.seen() {
		assert.True(t, strings.HasPrefix(r, "GET "), r)
		assert.True(t, strings.HasSuffix(r, "auth=token "+testToken), r)
	}
}

func TestHTTPRunRefusesWhenTheGuardFails(t *testing.T) {
	s := newAPIServer(t, map[string]string{"/api/user": `{"team":"admins"}`, "/x": `{}`})
	p := httpPopulate(s, systemmodel.HTTPObserve{
		Guard: &systemmodel.HTTPGuard{Requests: map[string]string{"": "/api/user"}, Check: cuePredicate(t, `{in: _, ok: in.team == "readers", reason: "need a reader token"}`)},
		Runs:  []systemmodel.HTTPRun{{Get: "/x"}},
	})
	_, err := Run(context.Background(), p, s.request())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "need a reader token")
	assert.Len(t, s.seen(), 1, "nothing is requested after the guard refuses")

	// A check that cannot be evaluated (no team at all) is not passed.
	s = newAPIServer(t, map[string]string{"/api/user": `{}`, "/x": `{}`})
	p = httpPopulate(s, systemmodel.HTTPObserve{
		Guard: &systemmodel.HTTPGuard{Requests: map[string]string{"": "/api/user"}, Check: cuePredicate(t, `{in: _, ok: in.team == "readers"}`)},
		Runs:  []systemmodel.HTTPRun{{Get: "/x"}},
	})
	_, err = Run(context.Background(), p, s.request())
	require.Error(t, err)
	assert.Len(t, s.seen(), 1)
}

func TestHTTPRunNeedsItsToken(t *testing.T) {
	s := newAPIServer(t, map[string]string{"/x": `{}`})
	p := httpPopulate(s, systemmodel.HTTPObserve{
		Auth: &systemmodel.HTTPAuth{Env: "UNSET_TOKEN", Header: "Authorization"},
		Runs: []systemmodel.HTTPRun{{Get: "/x"}},
	})
	_, err := Run(context.Background(), p, s.request())
	require.ErrorContains(t, err, "UNSET_TOKEN is not set")
	assert.Empty(t, s.seen())
}

// A non-2xx response is a failed input that keeps the status, not the body;
// a missing records path is one too, so an API that changes shape is not
// read as "none here".
func TestHTTPRunFailedInputsKeepNoBody(t *testing.T) {
	s := newAPIServer(t, map[string]string{"/ok": `{"items":[{"n":1}]}`, "/odd": `{"other":[]}`})
	p := httpPopulate(s, systemmodel.HTTPObserve{Runs: []systemmodel.HTTPRun{
		{Get: "/missing", Records: "items"}, {Get: "/ok", Records: "items"}, {Get: "/odd", Records: "items"},
	}})
	res, err := Run(context.Background(), p, s.request())
	require.NoError(t, err)
	require.Len(t, res.Failed, 2)
	assert.Equal(t, "GET /missing", res.Failed[0].Request)
	assert.Equal(t, "HTTP 404 Not Found", res.Failed[0].Error)
	assert.NotContains(t, res.Failed[0].Err().Error(), "secret-body")
	assert.Contains(t, res.Failed[1].Error, "response has no items")
	assert.Len(t, res.Records, 1)

	p.FailFast = true
	_, err = Run(context.Background(), p, s.request())
	require.ErrorContains(t, err, "GET /missing")
}

// A record that still has a rejected key fails the whole populate, and a
// transform error names paths, never the values involved.
func TestHTTPRunFailsClosedOnSecrets(t *testing.T) {
	s := newAPIServer(t, map[string]string{
		"/state": `{"resources":[{"urn":"u1","outputs":{"key":{"ciphertext":"c1pher"}}}]}`,
	})
	p := httpPopulate(s, systemmodel.HTTPObserve{
		Runs:       []systemmodel.HTTPRun{{Get: "/state", Records: "resources"}},
		RejectKeys: []string{"ciphertext"},
	})
	_, err := Run(context.Background(), p, s.request())
	require.ErrorContains(t, err, `rejected key "ciphertext"`)
	assert.NotContains(t, err.Error(), "c1pher")

	p.HTTP.Runs[0].Transform = cueTransform(t, `{in: _, out: {n: in.outputs.key.ciphertext & int}}`)
	res, err := Run(context.Background(), p, s.request())
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
	assert.Contains(t, res.Failed[0].Error, "transform failed at ")
	assert.NotContains(t, res.Failed[0].Error, "c1pher")
}

func TestHTTPRunRefusesRedirectsToAnotherOrigin(t *testing.T) {
	other := newAPIServer(t, map[string]string{"/x": `{"stolen":true}`})
	s := newAPIServer(t, map[string]string{"/x": "redirect:" + other.URL + "/x"})
	p := httpPopulate(s, systemmodel.HTTPObserve{Runs: []systemmodel.HTTPRun{{Get: "/x"}}})
	res, err := Run(context.Background(), p, s.request())
	require.NoError(t, err)
	require.Len(t, res.Failed, 1)
	assert.Contains(t, res.Failed[0].Error, "redirect to another origin")
	assert.Empty(t, other.seen(), "the token never reaches the other origin")
}

func TestHTTPRunRejectsInputsThatLeaveThePath(t *testing.T) {
	s := newAPIServer(t, map[string]string{"/list": `{"items":["ok","../admin"]}`})
	p := httpPopulate(s, systemmodel.HTTPObserve{
		Each: &systemmodel.HTTPEnumeration{Get: "/list", List: "items"},
		Runs: []systemmodel.HTTPRun{{Get: "/x/{{item}}"}},
	})
	_, err := Run(context.Background(), p, s.request())
	require.ErrorContains(t, err, `"../admin"`)
	assert.Len(t, s.seen(), 1, "no run is sent")
}

func TestHTTPRunHonoursTheStagingLimit(t *testing.T) {
	s := newAPIServer(t, map[string]string{"/big": `{"items":["` + strings.Repeat("a", 2048) + `"]}`})
	p := httpPopulate(s, systemmodel.HTTPObserve{Runs: []systemmodel.HTTPRun{{Get: "/big"}}})
	req := s.request()
	req.Limits = ingestprep.Limits{StagingBytes: 1024}
	_, err := Run(context.Background(), p, req)
	require.ErrorContains(t, err, "staging byte limit")
}

// A guard with named requests sends them in name order and checks all their
// responses together, so a check can join them.
func TestHTTPRunChecksNamedGuardRequestsTogether(t *testing.T) {
	routes := map[string]string{
		"/api/user":  `{"tokenInfo":{"organization":"acme","name":"pudl"}}`,
		"/api/roles": `{"roles":[{"set":"s1"}]}`,
		"/api/sets":  `{"sets":[{"id":"s1","permissions":["stack:read"]},{"id":"s2","permissions":["stack:write"]}]}`,
		"/x":         `{}`,
	}
	check := `{in: _, ok: in.user.tokenInfo.name == "pudl" &&
		[for s in in.sets.sets if s.id == in.roles.roles[0].set {s}][0].permissions == ["stack:read"],
		reason: "need a read-only token"}`
	guard := func() *systemmodel.HTTPGuard {
		return &systemmodel.HTTPGuard{
			Requests: map[string]string{"user": "/api/user", "roles": "/api/roles", "sets": "/api/sets"},
			Check:    cuePredicate(t, check),
		}
	}
	s := newAPIServer(t, routes)
	p := httpPopulate(s, systemmodel.HTTPObserve{Guard: guard(), Runs: []systemmodel.HTTPRun{{Get: "/x"}}})
	_, err := Run(context.Background(), p, s.request())
	require.NoError(t, err)
	var order []string
	for _, r := range s.seen() {
		order = append(order, strings.Fields(r)[1])
	}
	assert.Equal(t, []string{"/api/roles", "/api/sets", "/api/user", "/x"}, order)

	routes["/api/roles"] = `{"roles":[{"set":"s2"}]}`
	s = newAPIServer(t, routes)
	p = httpPopulate(s, systemmodel.HTTPObserve{Guard: guard(), Runs: []systemmodel.HTTPRun{{Get: "/x"}}})
	_, err = Run(context.Background(), p, s.request())
	require.ErrorContains(t, err, "need a read-only token")
	assert.Len(t, s.seen(), 3, "nothing is requested after the guard refuses")

	delete(routes, "/api/sets")
	s = newAPIServer(t, routes)
	p = httpPopulate(s, systemmodel.HTTPObserve{Guard: guard(), Runs: []systemmodel.HTTPRun{{Get: "/x"}}})
	_, err = Run(context.Background(), p, s.request())
	require.ErrorContains(t, err, "guard request sets (GET /api/sets): HTTP 404")
	assert.Len(t, s.seen(), 2, "a failed guard request stops the populate")
}
