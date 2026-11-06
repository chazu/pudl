package systemmodel

import (
	"errors"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"
)

// Predicate is an HTTP guard's `check: {in: _, ok: bool, reason?: string}`:
// ok is evaluated with in bound to a response. Like a Transform, it stays a
// CUE value because ok depends on in.
type Predicate struct {
	value cue.Value
}

var (
	predicateOK     = cue.ParsePath("ok")
	predicateReason = cue.ParsePath("reason")
)

// NewPredicate wraps a CUE `{in: _, ok: ...}` value.
func NewPredicate(v cue.Value) (*Predicate, error) {
	if err := v.Err(); err != nil {
		return nil, err
	}
	if !v.LookupPath(predicateOK).Exists() {
		return nil, fmt.Errorf("ok is required")
	}
	return &Predicate{value: v}, nil
}

// Eval reports whether ok is true for the response. A check that cannot be
// evaluated (a missing field, say) is not passed. reason is the declared
// reason, else a description naming only CUE paths, never values, because
// the response may hold secrets.
func (p *Predicate) Eval(response any) (ok bool, reason string) {
	declared := ""
	filled, err := fillIn(p.value, response)
	if err == nil {
		_ = filled.LookupPath(predicateReason).Decode(&declared)
		err = filled.LookupPath(predicateOK).Decode(&ok)
	}
	if err == nil && ok {
		return true, ""
	}
	if declared != "" {
		return false, declared
	}
	if err != nil {
		return false, "check could not be evaluated" + ErrorPaths(err)
	}
	return false, "check.ok is false"
}

// ErrorPaths describes where a CUE evaluation failed, by path only: CUE
// messages quote the values involved, which may be secrets. Empty when err
// names no path.
func ErrorPaths(err error) string {
	// A wrapped CUE error (fmt.Errorf %w) is unwrapped first: Errors would
	// promote the wrapper, which has no path.
	var cueErr cueerrors.Error
	if errors.As(err, &cueErr) {
		err = cueErr
	}
	var paths []string
	seen := map[string]bool{}
	for _, e := range cueerrors.Errors(err) {
		path := strings.Join(e.Path(), ".")
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return ""
	}
	return " at " + strings.Join(paths, ", ")
}
