package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chazu/pudl/internal/systemmodel"
)

func TestCommandTreeJSONExposesParitySurface(t *testing.T) {
	tree := commandTreeJSON(rootCmd)
	run, ok := helpChild(tree, "run [<model>]")
	require.True(t, ok, "root help must expose the run command")
	assert.True(t, hasHelpFlag(run, "populate"))
	assert.True(t, hasHelpFlag(run, "require-approval"))
	report, ok := helpChild(run, "report [run-id]")
	require.True(t, ok, "run help must expose durable reports")
	assert.Equal(t, "custom", report.Args)

	model, ok := helpChild(tree, "model")
	require.True(t, ok)
	describe, ok := helpChild(model, "describe <name>")
	require.True(t, ok)
	assert.Equal(t, "custom", describe.Args)
}

func TestScaffoldInputAndModelText(t *testing.T) {
	plugin, err := parsePluginSpec(" plugin:k8s ")
	require.NoError(t, err)
	assert.Equal(t, "k8s", plugin)
	_, err = parsePluginSpec("k8s")
	require.Error(t, err)

	input, err := parseKeyValueInputs([]string{"namespace=default", `kinds=["pods","services"]`, "all_namespaces=true"})
	require.NoError(t, err)
	assert.Equal(t, "default", input["namespace"])
	assert.Equal(t, []any{"pods", "services"}, input["kinds"])
	assert.Equal(t, true, input["all_namespaces"])

	source, err := renderModelScaffold("cluster pods", "k8s", input)
	require.NoError(t, err)
	assert.Contains(t, source, "package models")
	assert.Contains(t, source, `import sm "pudl.schemas/pudl/systemmodel@v0"`)
	assert.Contains(t, source, `name: "cluster pods"`)
	assert.Contains(t, source, `plugin: "k8s"`)
}

func TestPersistRunReportUsesTheRunCatalog(t *testing.T) {
	cat := newRunCatalog(t.TempDir())
	defer cat.Close()

	report := &RunReport{
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

func TestDescribeSystemModelIncludesRuntimeContract(t *testing.T) {
	m := &systemmodel.SystemModel{
		Name:      "pods",
		Populate:  systemmodel.Populate{Plugin: "k8s", Input: map[string]any{"inventory": true}, Differential: false},
		Plugins:   []systemmodel.PluginDef{{Name: "k8s", Digest: "sha256:abc"}},
		Desired:   []map[string]any{{"name": "web"}},
		Checks:    []systemmodel.Check{{Name: "healthy"}},
		DependsOn: []string{"network"},
		Converge:  &systemmodel.PluginPlan{Plugin: "k8s", Input: map[string]any{"namespace": "default"}},
		Freshness: &systemmodel.Freshness{Every: "10m", Drift: true},
	}

	d := describeSystemModel(m, func(name string) (map[string]any, error) {
		return map[string]any{"name": name, "capabilities": []any{"observe", "plan"}}, nil
	})
	assert.Equal(t, "pods", d.Name)
	assert.Equal(t, "k8s", d.Populate["plugin"])
	assert.Len(t, d.Desired, 1)
	assert.Len(t, d.Checks, 1)
	assert.Equal(t, []string{"network"}, d.DependsOn)
	assert.NotNil(t, d.Converge)
	assert.NotNil(t, d.Freshness)
	require.Len(t, d.Plugins, 1)
	assert.NotNil(t, d.Plugins[0].(map[string]any)["discovery"])
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

func TestRunReportMarkdownShowsFailureContext(t *testing.T) {
	text := (&RunReport{Model: "pods", RunID: "run_1", Mode: "observe-only", OK: false, Error: "boom"}).markdown()
	assert.True(t, strings.Contains(text, "status: FAILED"))
}
