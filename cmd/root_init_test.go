package cmd

import "testing"

func TestGlobalAutoInitCommandBoundary(t *testing.T) {
	for _, command := range []string{"help", "version", "init", "repo", "--help", "-h", "--version", "-v"} {
		if shouldAutoInitializeGlobal(command) {
			t.Errorf("%q should not auto-initialize global state", command)
		}
	}
	if !shouldAutoInitializeGlobal("list") {
		t.Error("ordinary global-mode command should auto-initialize global state")
	}
}
