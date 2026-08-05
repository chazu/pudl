package cmd

import (
	"strings"
	"testing"
)

func TestShellIntegrationsUseCurrentSchemaPath(t *testing.T) {
	for name, snippet := range map[string]string{
		"bash": bashIntegration,
		"zsh":  zshIntegration,
		"fish": fishIntegration,
	} {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(snippet, "pudl git") {
				t.Fatal("shell integration references removed pudl git command")
			}
			if !strings.Contains(snippet, `cd "$PUDL_SCHEMA_PATH"`) {
				t.Fatal("shell integration does not navigate to PUDL_SCHEMA_PATH")
			}
		})
	}
}
