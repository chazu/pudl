package systemmodel

import (
	"fmt"
	"regexp"
)

// EmptyPatterns compiles populate.empty_when (Go RE2 syntax). A pattern that
// does not compile is an error naming the field, so loading the model (and
// `pudl model validate`) reports it before anything runs.
func (p Populate) EmptyPatterns() ([]*regexp.Regexp, error) {
	return compileStderrPatterns("populate.empty_when", "failure", p.EmptyWhen)
}

// FailPatterns compiles populate.fail_when (Go RE2 syntax), with the same
// errors as EmptyPatterns.
func (p Populate) FailPatterns() ([]*regexp.Regexp, error) {
	return compileStderrPatterns("populate.fail_when", "run", p.FailWhen)
}

// compileStderrPatterns compiles one list of stderr patterns. An empty
// pattern is an error: it would match every run of the kind named.
func compileStderrPatterns(field, matches string, patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for i, pattern := range patterns {
		if pattern == "" {
			return nil, fmt.Errorf("%s[%d] is empty, so it would match every %s", field, i, matches)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%s[%d] %q is not a valid regular expression: %w", field, i, pattern, err)
		}
		out = append(out, re)
	}
	return out, nil
}
