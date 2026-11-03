package populate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Why a populate's fallback was used.
const (
	// FallbackFailedInput: a run failed with stderr matching fallback.when.
	FallbackFailedInput = "failed_input"
	// FallbackEachFailed: the each command failed with stderr matching
	// fallback.when.
	FallbackEachFailed = "each_failed"
	// FallbackNoInputs: the each command listed no inputs.
	FallbackNoInputs = "no_inputs"
)

// FallbackUse records that a populate's fallback answered instead of its own
// runs, and why.
type FallbackUse struct {
	Reason string `json:"reason"`
	// Pattern is the fallback.when pattern that matched, and StderrLine the
	// stderr line it matched in; empty for no_inputs.
	Pattern    string `json:"pattern,omitempty"`
	StderrLine string `json:"stderr_line,omitempty"`
	// Trigger is the failed run or each command; nil for no_inputs.
	Trigger *FailedInput `json:"trigger,omitempty"`
}

// Summary describes the trigger for a person in one line.
func (u FallbackUse) Summary() string {
	switch u.Reason {
	case FallbackNoInputs:
		return "each listed no inputs"
	case FallbackEachFailed:
		return fmt.Sprintf("each (%s) failed: %s", strings.Join(u.Trigger.Argv, " "), u.StderrLine)
	default:
		return fmt.Sprintf("%s failed: %s", u.Trigger.Label(), u.StderrLine)
	}
}

// eachError is a failed each command, kept whole so its stderr can trigger a
// fallback.
type eachError struct{ failure FailedInput }

func (e *eachError) Error() string {
	if e.failure.Stderr != "" {
		return fmt.Sprintf("%s\nstderr (last %d bytes):\n%s", e.failure.Error, stderrTail, e.failure.Stderr)
	}
	return e.failure.Error
}

// eachFallback reports whether a failed enumeration triggers the fallback.
func eachFallback(when []*regexp.Regexp, err error) (*FallbackUse, bool) {
	var ee *eachError
	if when == nil || !errors.As(err, &ee) {
		return nil, false
	}
	pattern, line, ok := matchStderr(when, &ee.failure)
	if !ok {
		return nil, false
	}
	f := ee.failure
	return &FallbackUse{Reason: FallbackEachFailed, Pattern: pattern, StderrLine: line, Trigger: &f}, true
}
