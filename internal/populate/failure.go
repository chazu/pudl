package populate

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// FailedInput is a run that failed: the input whose observation is missing
// from the populate.
type FailedInput struct {
	// Run is the run's index after enumeration.
	Run int `json:"run"`
	// Item is the enumerated input the run was for; empty without `each`.
	Item string   `json:"item,omitempty"`
	Argv []string `json:"argv"`
	// ExitStatus is the command's exit code: 0 when it succeeded but printed
	// something other than JSON objects, -1 when it did not exit on its own
	// (a timeout or signal) or could not start.
	ExitStatus int    `json:"exit_status"`
	Error      string `json:"error"`
	// Stderr is the end of the command's stderr.
	Stderr string `json:"stderr_tail,omitempty"`
	// exited: the command exited non-zero on its own, so its stderr may
	// declare the input empty (see EmptyInput).
	exited bool
}

// Err is the failure as one error: the run, its argv, what went wrong, and
// the end of its stderr.
func (f FailedInput) Err() error {
	msg := fmt.Sprintf("populate run %d (%s): %s", f.Run, strings.Join(f.Argv, " "), f.Error)
	if f.Stderr != "" {
		msg += fmt.Sprintf("\nstderr (last %d bytes):\n%s", stderrTail, f.Stderr)
	}
	return errors.New(msg)
}

// Label names the input for a person: its enumerated item, else its argv.
func (f FailedInput) Label() string {
	if f.Item != "" {
		return f.Item
	}
	return strings.Join(f.Argv, " ")
}

func exitStatus(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
