package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBinaryExcludesTestPackages guards the release binary against linking test
// helpers: a non-_test.go file that imports "testing" or testify drags both into
// every pudl build.
func TestBinaryExcludesTestPackages(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not available")
	}
	out, err := exec.Command(goTool, "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, pkg := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if pkg == "testing" || strings.HasPrefix(pkg, "github.com/stretchr/testify") {
			t.Errorf("pudl binary depends on test package %s", pkg)
		}
	}
}
