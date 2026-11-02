package systemmodel

import (
	"fmt"
	"regexp"
)

// EmptyPatterns compiles populate.empty_when (Go RE2 syntax). A pattern that
// does not compile is an error naming the field, so loading the model (and
// `pudl model validate`) reports it before anything runs.
func (p Populate) EmptyPatterns() ([]*regexp.Regexp, error) {
	if len(p.EmptyWhen) == 0 {
		return nil, nil
	}
	out := make([]*regexp.Regexp, 0, len(p.EmptyWhen))
	for i, pattern := range p.EmptyWhen {
		if pattern == "" {
			return nil, fmt.Errorf("populate.empty_when[%d] is empty, so it would match every failure", i)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("populate.empty_when[%d] %q is not a valid regular expression: %w", i, pattern, err)
		}
		out = append(out, re)
	}
	return out, nil
}
