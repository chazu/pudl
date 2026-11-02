package populate

import (
	"regexp"
	"strings"
)

// EmptyInput is a run that exited non-zero with stderr matching one of the
// populate's empty_when patterns: a definite answer that the input has no
// resources (for example an API that is disabled), not missing evidence. It
// is observed with zero records and leaves the observation complete.
type EmptyInput struct {
	// Run is the run's index after enumeration.
	Run int `json:"run"`
	// Item is the enumerated input the run was for; empty without `each`.
	Item       string   `json:"item,omitempty"`
	Argv       []string `json:"argv"`
	ExitStatus int      `json:"exit_status"`
	// Pattern is the empty_when pattern that matched.
	Pattern string `json:"pattern"`
	// Reason is the stderr line the pattern matched in.
	Reason string `json:"reason"`
}

// Label names the input for a person: its enumerated item, else its argv.
func (e EmptyInput) Label() string {
	if e.Item != "" {
		return e.Item
	}
	return strings.Join(e.Argv, " ")
}

// matchEmpty reports whether a failed run is an empty input: it exited
// non-zero on its own (not a timeout, a signal, or undecodable stdout) and
// one of the patterns matches its stderr. The first matching pattern wins.
func matchEmpty(patterns []*regexp.Regexp, f *FailedInput) (EmptyInput, bool) {
	if !f.exited {
		return EmptyInput{}, false
	}
	for _, re := range patterns {
		loc := re.FindStringIndex(f.Stderr)
		if loc == nil {
			continue
		}
		return EmptyInput{
			Run: f.Run, Item: f.Item, Argv: f.Argv, ExitStatus: f.ExitStatus,
			Pattern: re.String(), Reason: lineAt(f.Stderr, loc[0]),
		}, true
	}
	return EmptyInput{}, false
}

// lineAt returns the trimmed line of s containing byte offset i.
func lineAt(s string, i int) string {
	start := strings.LastIndexByte(s[:i], '\n') + 1
	end := len(s)
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		end = i + j
	}
	return strings.TrimSpace(s[start:end])
}
