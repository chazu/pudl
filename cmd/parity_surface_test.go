package cmd

import (
	"testing"

	"github.com/chazu/pudl/internal/checkreport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandTreeJSONExposesParitySurface(t *testing.T) {
	tree := commandTreeJSON(rootCmd)
	check, ok := helpChild(tree, "check <model>...")
	require.True(t, ok, "root help must expose the check command")
	assert.True(t, hasHelpFlag(check, "populate"))
	assert.True(t, hasHelpFlag(check, "detailed-exitcode"))
	report, ok := helpChild(tree, "report [ID]")
	require.True(t, ok, "root help must expose durable reports")
	assert.Equal(t, "custom", report.Args)

	model, ok := helpChild(tree, "model")
	require.True(t, ok)
	describe, ok := helpChild(model, "show <name>")
	require.True(t, ok)
	assert.Equal(t, "custom", describe.Args)
}

func TestPersistRunReportUsesTheRunCatalog(t *testing.T) {
	cat := newRunCatalog(t.TempDir())
	defer cat.Close()

	report := &checkreport.ModelReport{
		RunID: "run_parity", Model: "pods", Mode: "observe-only", OK: false,
		Error: "observer failed after startup",
	}
	require.True(t, persistRunReport(cat, report, false))

	db, err := cat.required()
	require.NoError(t, err)
	stored, err := db.GetRunReport("run_parity")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "pods", stored.Model)
	assert.Contains(t, string(stored.Report), "observer failed after startup")
}

func helpChild(parent commandHelpJSON, use string) (commandHelpJSON, bool) {
	for _, child := range parent.Subcommands {
		if child.Use == use {
			return child, true
		}
	}
	return commandHelpJSON{}, false
}

func hasHelpFlag(command commandHelpJSON, name string) bool {
	for _, flag := range command.Flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

func TestModelScaffoldDefaultsToCommandObserve(t *testing.T) {
	source, err := renderCommandScaffold("cluster pods", nil)
	require.NoError(t, err)
	assert.Contains(t, source, "package models")
	assert.Contains(t, source, `import sm "pudl.schemas/pudl/systemmodel@v0"`)
	assert.Contains(t, source, `name: "cluster pods"`)
	assert.Contains(t, source, `runs: [{argv: ["echo","[]"]}]`)
	assert.Contains(t, source, "Replace with a command that prints JSON records")

	source, err = renderCommandScaffold("fw", []string{"gcloud", "list"})
	require.NoError(t, err)
	assert.Contains(t, source, `runs: [{argv: ["gcloud","list"]}]`)
	assert.NotContains(t, source, "Replace with")
}
