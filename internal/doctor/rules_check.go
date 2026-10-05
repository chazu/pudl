package doctor

import (
	"fmt"
	"strings"

	"github.com/chazu/pudl/internal/datalog"
)

// CheckRulesAt loads the Datalog rules on the given search paths and reports
// any that are invalid. Queries and model checks refuse to evaluate a relation
// that depends on an invalid rule, so each one is an error here.
func CheckRulesAt(paths ...string) *CheckResult {
	rules, err := datalog.LoadRulesFromPaths(paths...)
	if err != nil {
		return &CheckResult{
			Status:  "error",
			Message: "Datalog rule files do not load",
			Details: err.Error(),
			Fix:     "Fix the CUE syntax in the reported rule file",
		}
	}

	problems := datalog.RuleSetProblems(rules)
	if len(problems) == 0 {
		return &CheckResult{
			Status:  "ok",
			Message: fmt.Sprintf("%d Datalog rule(s) load cleanly", len(rules)),
		}
	}

	return &CheckResult{
		Status:  "error",
		Message: fmt.Sprintf("%d Datalog rule problem(s); queries depending on them fail", len(problems)),
		Details: strings.Join(problems, "\n"),
		Fix:     "Correct each rule at the reported position (see docs/datalog.md)",
	}
}
