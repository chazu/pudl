package populate

import (
	"fmt"
	"regexp"
)

// matchFail returns a failed input for a run that exited 0 but whose stderr
// matches one of the fail_when patterns (the first match wins), else nil. Its
// exit status is 0 and it never counts as an empty input.
func matchFail(patterns []*regexp.Regexp, stderr string) *FailedInput {
	for _, re := range patterns {
		if loc := re.FindStringIndex(stderr); loc != nil {
			return &FailedInput{
				Error:  fmt.Sprintf("exited 0, but stderr matched fail_when %q: %s", re.String(), lineAt(stderr, loc[0])),
				Stderr: stderr,
			}
		}
	}
	return nil
}
