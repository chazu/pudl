package populate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/chazu/pudl/internal/systemmodel"
)

// errHTTPPopulate marks a failure of the populate itself (the context ended,
// the byte budget ran out) as opposed to one request's failure.
var errHTTPPopulate = errors.New("http populate")

// runHTTP executes an #HTTPObserve populate: the guard, then the `each`
// enumeration if any, then each run's GET. Responses are read under the
// staging byte budget and decoded in memory; nothing is written to disk.
//
// A run whose request fails (a transport error, a non-2xx status, a body
// that is not JSON, a record the transform rejects) is a failed input, and
// the other runs go on; with FailFast it fails the populate. A missing token,
// a guard that does not pass, a failed enumeration, a record that keeps a
// rejected key, the byte budget and the context ending fail the populate.
func runHTTP(ctx context.Context, p systemmodel.Populate, req Request) (Result, error) {
	h := p.HTTP
	limits, err := req.Limits.Resolve()
	if err != nil {
		return Result{}, err
	}
	timeout := DefaultTimeout
	if p.Timeout != "" {
		if timeout, err = time.ParseDuration(p.Timeout); err != nil {
			return Result{}, fmt.Errorf("populate timeout: %w", err)
		}
	}
	c, err := newHTTPSource(ctx, h, req, timeout, limits.StagingBytes)
	if err != nil {
		return Result{}, err
	}
	if h.Guard != nil {
		if err := c.guard(h.Guard); err != nil {
			return Result{}, err
		}
	}
	res := Result{Records: make([]any, 0)}
	runs := h.Runs
	var items []string
	if h.Each != nil {
		if items, err = c.enumerate(*h.Each); err != nil {
			return Result{}, fmt.Errorf("populate each (GET %s): %w", h.Each.Get, err)
		}
		res.Inputs = items
		runs = make([]systemmodel.HTTPRun, 0, len(items)*len(h.Runs))
		for _, item := range items {
			for _, run := range h.Runs {
				runs = append(runs, run.ForItem(item))
			}
		}
	}
	res.Runs = len(runs)
	for i, run := range runs {
		records, failure, err := c.observe(run)
		if err != nil {
			return Result{}, fmt.Errorf("populate run %d (GET %s): %w", i, run.Get, err)
		}
		if failure != nil {
			failure.Run, failure.Request = i, "GET "+run.Get
			if items != nil {
				failure.Item = items[i/len(h.Runs)]
			}
			if p.FailFast {
				return Result{}, failure.Err()
			}
			res.Failed = append(res.Failed, *failure)
			continue
		}
		res.Records = append(res.Records, records...)
	}
	return res, nil
}

// httpSource sends the GET requests of one populate.
type httpSource struct {
	ctx     context.Context
	spec    *systemmodel.HTTPObserve
	client  *http.Client
	token   string
	timeout time.Duration
	budget  *byteBudget
}

func newHTTPSource(ctx context.Context, h *systemmodel.HTTPObserve, req Request, timeout time.Duration, staging int64) (*httpSource, error) {
	base, err := url.Parse(h.Base)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("populate base %q is not an https origin", h.Base)
	}
	c := &httpSource{ctx: ctx, spec: h, timeout: timeout, budget: &byteBudget{remaining: staging}}
	if h.Auth != nil {
		getenv := req.Getenv
		if getenv == nil {
			getenv = os.Getenv
		}
		if c.token = getenv(h.Auth.Env); c.token == "" {
			return nil, fmt.Errorf("populate auth: environment variable %s is not set", h.Auth.Env)
		}
	}
	client := http.Client{}
	if req.HTTPClient != nil {
		client = *req.HTTPClient
	}
	// The token goes to base alone: a redirect elsewhere is refused.
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || r.URL.Host != base.Host {
			return fmt.Errorf("redirect to another origin (%s) refused", r.URL.Host)
		}
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return nil
	}
	c.client = &client
	return c, nil
}

// guard sends the guard's requests in name order and evaluates its check over
// their responses: the one response itself, or each by name. A failed
// request or a check that does not pass fails the populate.
func (c *httpSource) guard(g *systemmodel.HTTPGuard) error {
	if path, ok := g.Single(); ok {
		body, err := c.get(path)
		if err != nil {
			return fmt.Errorf("populate guard (GET %s): %w", path, err)
		}
		if ok, reason := g.Check.Eval(body); !ok {
			return fmt.Errorf("populate guard (GET %s) refused: %s", path, reason)
		}
		return nil
	}
	responses := make(map[string]any, len(g.Requests))
	for _, name := range g.Names() {
		path := g.Requests[name]
		body, err := c.get(path)
		if err != nil {
			return fmt.Errorf("populate guard request %s (GET %s): %w", name, path, err)
		}
		responses[name] = body
	}
	if ok, reason := g.Check.Eval(responses); !ok {
		return fmt.Errorf("populate guard (%d requests) refused: %s", len(responses), reason)
	}
	return nil
}

// statusError is a non-2xx response; its body is not kept.
type statusError struct{ code int }

func (e statusError) Error() string {
	return fmt.Sprintf("HTTP %d %s", e.code, http.StatusText(e.code))
}

// get sends one GET to base + path and decodes the JSON response, numbers
// exact. Errors wrapping errHTTPPopulate fail the populate; the others are
// the request's own.
func (c *httpSource) get(path string) (any, error) {
	u, err := c.spec.URL(path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/json")
	for name, value := range c.spec.Headers {
		r.Header.Set(name, value)
	}
	if a := c.spec.Auth; a != nil {
		value := c.token
		if a.Scheme != "" {
			value = a.Scheme + " " + c.token
		}
		r.Header.Set(a.Header, value)
	}
	resp, err := c.client.Do(r)
	if err != nil {
		if c.ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", errHTTPPopulate, c.ctx.Err())
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, statusError{code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.budget.remaining+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > c.budget.remaining {
		c.budget.exceeded = true
		return nil, fmt.Errorf("%w: response exceeds the staging byte limit", errHTTPPopulate)
	}
	c.budget.remaining -= int64(len(body))
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, errors.New("response is not JSON")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("response is not one JSON value")
	}
	return v, nil
}
