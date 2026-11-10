package systemmodel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
)

// HTTPObserve is the #HTTPObserve arm: GET requests to one HTTPS origin whose
// responses hold JSON records. The fields shared with commands (schema,
// timeout, fail_fast) stay on Populate.
type HTTPObserve struct {
	Base    string            `json:"base"`
	Headers map[string]string `json:"headers,omitempty"`
	Auth    *HTTPAuth         `json:"auth,omitempty"`
	Guard   *HTTPGuard        `json:"guard,omitempty"`
	Each    *HTTPEnumeration  `json:"each,omitempty"`
	Runs    []HTTPRun         `json:"runs"`
	// RejectKeys fail the populate when a kept record still has one of these
	// keys at any depth.
	RejectKeys []string `json:"reject_keys,omitempty"`
}

// HTTPAuth sends the token held by environment variable Env as
// `Header: Scheme token`.
type HTTPAuth struct {
	Env    string `json:"env"`
	Header string `json:"header"`
	Scheme string `json:"scheme"`
}

// HTTPEnumeration lists the inputs every run is repeated for.
type HTTPEnumeration struct {
	Get  string    `json:"get"`
	List string    `json:"list"`
	Page *HTTPPage `json:"page,omitempty"`
	// Item maps one listed element to its input; nil when elements are
	// strings. Read from the CUE value; it is not recorded.
	Item *Transform `json:"-"`
}

// HTTPPage follows a continuation token: the response's Next field, sent
// back as query parameter Param.
type HTTPPage struct {
	Next  string `json:"next"`
	Param string `json:"param"`
}

// HTTPRun is one GET request; Records is the field path of the record array
// in its response.
type HTTPRun struct {
	Get     string `json:"get"`
	Records string `json:"records,omitempty"`
	// RecordsOptional reads a response without the Records path as no
	// records instead of a failed input.
	RecordsOptional bool           `json:"records_optional,omitempty"`
	Set             map[string]any `json:"set,omitempty"`
	// Transform is read from the CUE value; it is not recorded.
	Transform *Transform `json:"-"`
}

// ForItem returns the run for one enumerated input: the placeholder replaced
// in get and in set's string values. The receiver is not modified.
func (r HTTPRun) ForItem(item string) HTTPRun {
	out := HTTPRun{Get: strings.ReplaceAll(r.Get, ItemPlaceholder, item), Records: r.Records, RecordsOptional: r.RecordsOptional, Transform: r.Transform}
	if r.Set != nil {
		out.Set, _ = substituteItem(r.Set, item).(map[string]any)
	}
	return out
}

func (r HTTPRun) usesItem() bool {
	return strings.Contains(r.Get, ItemPlaceholder) || setUsesItem(r.Set)
}

// validItem is what an enumerated input may hold, so substituting it into a
// path can neither leave the path nor add a query.
var validItem = regexp.MustCompile(`^[A-Za-z0-9._~/-]+$`)

// ValidateItem rejects an input that could change a request beyond its
// placeholder.
func ValidateItem(item string) error {
	if !validItem.MatchString(item) || strings.Contains(item, "..") {
		return fmt.Errorf("input %q may hold only letters, digits and . _ ~ / -, and no \"..\"", item)
	}
	return nil
}

// URL resolves a request path against base, and refuses one that would reach
// another origin.
func (h *HTTPObserve) URL(path string) (*url.URL, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, fmt.Errorf("request path %q must start with a single /", path)
	}
	u, err := url.Parse(h.Base + path)
	if err != nil {
		return nil, fmt.Errorf("request path %q: %w", path, err)
	}
	base, err := url.Parse(h.Base)
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	if u.Scheme != "https" || u.Host != base.Host || u.User != nil {
		return nil, fmt.Errorf("request path %q leaves %s", path, h.Base)
	}
	return u, nil
}

// decodeHTTP reads the #HTTPObserve arm of a model's populate, with its
// guard check, item mapping and run transforms, and validates it.
func decodeHTTP(inst cue.Value, p *Populate) error {
	v := inst.LookupPath(cue.ParsePath("populate"))
	var h HTTPObserve
	if err := v.Decode(&h); err != nil {
		return fmt.Errorf("populate: %w", err)
	}
	if h.Guard != nil {
		g, err := decodeGuard(v.LookupPath(cue.ParsePath("guard")))
		if err != nil {
			return err
		}
		h.Guard = g
	}
	if h.Each != nil {
		if item := v.LookupPath(cue.ParsePath("each.item")); item.Exists() {
			t, err := NewTransform(item)
			if err != nil {
				return fmt.Errorf("populate.each.item: %w", err)
			}
			h.Each.Item = t
		}
	}
	runs := v.LookupPath(cue.ParsePath("runs"))
	for i := range h.Runs {
		t := runs.LookupPath(cue.MakePath(cue.Index(i), cue.Str("transform")))
		if !t.Exists() {
			continue
		}
		tr, err := NewTransform(t)
		if err != nil {
			return fmt.Errorf("populate.runs[%d].transform: %w", i, err)
		}
		h.Runs[i].Transform = tr
	}
	if err := h.validate(); err != nil {
		return fmt.Errorf("populate: %w", err)
	}
	// The command fields Decode filled from the HTTP shape mean nothing here.
	p.Runs, p.Each, p.HTTP = nil, nil, &h
	return nil
}

// validate checks what the schema cannot: every path stays on base, the
// auth header is not also a static header, and the item placeholder is used
// exactly when there is an enumeration.
func (h *HTTPObserve) validate() error {
	paths := []string{}
	if h.Guard != nil {
		for _, name := range h.Guard.Names() {
			paths = append(paths, h.Guard.Requests[name])
		}
	}
	if h.Each != nil {
		paths = append(paths, h.Each.Get)
	}
	for i, run := range h.Runs {
		uses := run.usesItem()
		switch {
		case h.Each != nil && !uses:
			return fmt.Errorf("run %d does not use %s, so `each` would repeat it identically", i, ItemPlaceholder)
		case h.Each == nil && uses:
			return fmt.Errorf("run %d uses %s but the populate declares no `each`", i, ItemPlaceholder)
		}
		paths = append(paths, strings.ReplaceAll(run.Get, ItemPlaceholder, "item"))
	}
	for _, path := range paths {
		if _, err := h.URL(path); err != nil {
			return err
		}
	}
	for name := range h.Headers {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "Host" || (h.Auth != nil && canonical == http.CanonicalHeaderKey(h.Auth.Header)) {
			return fmt.Errorf("headers may not set %s", canonical)
		}
	}
	return nil
}

// MarshalJSON records an HTTP populate in its #HTTPObserve shape, and a
// command populate as declared.
func (p Populate) MarshalJSON() ([]byte, error) {
	type command Populate
	if p.HTTP == nil {
		return json.Marshal(command(p))
	}
	return json.Marshal(struct {
		*HTTPObserve
		Schema              string `json:"schema,omitempty"`
		AllowSchemaFallback bool   `json:"allow_schema_fallback,omitempty"`
		Timeout             string `json:"timeout,omitempty"`
		FailFast            bool   `json:"fail_fast,omitempty"`
	}{p.HTTP, p.Schema, p.AllowSchemaFallback, p.Timeout, p.FailFast})
}
