// Package testenv isolates test binaries from the developer's real
// environment. Import it only from _test.go files.
package testenv

import (
	"fmt"
	"os"
	"testing"
)

// RunWithIsolatedHome runs m with HOME pointed at a fresh temporary directory,
// so a test that forgets t.Setenv("HOME", ...) cannot read or write the real
// ~/.pudl or ~/.mu. Tests that set HOME themselves still take precedence.
// Use it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testenv.RunWithIsolatedHome(m)) }
func RunWithIsolatedHome(m *testing.M) int {
	home, err := os.MkdirTemp("", "pudl-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testenv: create temp HOME: %v\n", err)
		return 1
	}
	defer os.RemoveAll(home)

	if err := os.Setenv("HOME", home); err != nil {
		fmt.Fprintf(os.Stderr, "testenv: set HOME: %v\n", err)
		return 1
	}
	return m.Run()
}
