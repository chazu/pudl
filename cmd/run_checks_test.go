package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckPasses(t *testing.T) {
	cases := []struct {
		expect string
		count  int
		want   bool
	}{
		{"empty", 0, true},
		{"empty", 1, false},
		{"empty", 5, false},
		{"nonempty", 0, false},
		{"nonempty", 1, true},
		{"bogus", 0, false}, // unknown expect never passes
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, checkPasses(tc.expect, tc.count),
			"checkPasses(%q,%d)", tc.expect, tc.count)
	}
}

func TestPrintChecks_FailSeverityGates(t *testing.T) {
	// a failing "fail" check -> failedFail true; failing "warn" -> false.
	assert.True(t, printChecks([]CheckResult{
		{Name: "a", Severity: "fail", Passed: false},
	}))
	assert.False(t, printChecks([]CheckResult{
		{Name: "b", Severity: "warn", Passed: false},
		{Name: "c", Severity: "info", Passed: true},
	}))
	assert.False(t, printChecks([]CheckResult{
		{Name: "d", Severity: "fail", Passed: true},
	}))
}
