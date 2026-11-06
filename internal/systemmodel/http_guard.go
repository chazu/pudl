package systemmodel

import (
	"encoding/json"
	"fmt"
	"sort"

	"cuelang.org/go/cue"
)

// HTTPGuard is the GET requests whose responses must pass Check before any
// other request is sent.
type HTTPGuard struct {
	// Requests are the guard's paths by name. A guard declared with one path
	// (`get: "/path"`) has one request named "", and check.in is its
	// response; with named paths (`get: {user: "/a", roles: "/b"}`),
	// check.in.<name> is each response.
	Requests map[string]string `json:"-"`
	// Check is read from the CUE value; it is not recorded.
	Check *Predicate `json:"-"`
}

// Single returns the path of a guard declared with one path.
func (g *HTTPGuard) Single() (string, bool) {
	path, ok := g.Requests[""]
	return path, ok && len(g.Requests) == 1
}

// Names returns the names of a guard's requests, sorted: the order they are
// sent in.
func (g *HTTPGuard) Names() []string {
	names := make([]string, 0, len(g.Requests))
	for name := range g.Requests {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MarshalJSON records the guard as declared: `get` as one path or a map of
// named paths. The check depends on in and is not recorded.
func (g HTTPGuard) MarshalJSON() ([]byte, error) {
	if path, ok := g.Single(); ok {
		return json.Marshal(map[string]any{"get": path})
	}
	return json.Marshal(map[string]any{"get": g.Requests})
}

// decodeGuard reads a populate's guard: its path or named paths, and its
// required check.
func decodeGuard(v cue.Value) (*HTTPGuard, error) {
	g := &HTTPGuard{}
	get := v.LookupPath(cue.ParsePath("get"))
	switch get.IncompleteKind() {
	case cue.StringKind:
		var path string
		if err := get.Decode(&path); err != nil {
			return nil, fmt.Errorf("populate.guard.get: %w", err)
		}
		g.Requests = map[string]string{"": path}
	default:
		if err := get.Decode(&g.Requests); err != nil {
			return nil, fmt.Errorf("populate.guard.get: %w", err)
		}
		if len(g.Requests) == 0 {
			return nil, fmt.Errorf("populate.guard.get names no request")
		}
		if _, ok := g.Requests[""]; ok {
			return nil, fmt.Errorf("populate.guard.get: a request name may not be empty")
		}
	}
	check := v.LookupPath(cue.ParsePath("check"))
	if !check.Exists() {
		return nil, fmt.Errorf("populate.guard.check is required")
	}
	pred, err := NewPredicate(check)
	if err != nil {
		return nil, fmt.Errorf("populate.guard.check: %w", err)
	}
	g.Check = pred
	return g, nil
}
